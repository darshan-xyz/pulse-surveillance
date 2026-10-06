provider "aws" {
  region = var.aws_region
}

# 1. VPC Configuration
module "vpc" {
  source  = "terraform-aws-modules/vpc/aws"
  version = "5.1.2"

  name = "pulse-vpc"
  cidr = "10.0.0.0/16"

  azs             = ["${var.aws_region}a", "${var.aws_region}b"]
  private_subnets = ["10.0.1.0/24", "10.0.2.0/24"]
  public_subnets  = ["10.0.101.0/24", "10.0.102.0/24"]

  enable_nat_gateway = true
  single_nat_gateway = true
}

# 2. Security Groups
resource "aws_security_group" "alb_sg" {
  name        = "pulse-alb-sg"
  description = "Allow inbound WSS"
  vpc_id      = module.vpc.vpc_id

  ingress {
    from_port   = 443
    to_port     = 443
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}

resource "aws_security_group" "ecs_sg" {
  name        = "pulse-ecs-sg"
  vpc_id      = module.vpc.vpc_id

  ingress {
    from_port       = 9091
    to_port         = 9091
    protocol        = "tcp"
    security_groups = [aws_security_group.alb_sg.id]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}

resource "aws_security_group" "db_cache_sg" {
  name   = "pulse-db-sg"
  vpc_id = module.vpc.vpc_id

  ingress {
    from_port       = 5432
    to_port         = 5432
    protocol        = "tcp"
    security_groups = [aws_security_group.ecs_sg.id]
  }
  
  ingress {
    from_port       = 6379
    to_port         = 6379
    protocol        = "tcp"
    security_groups = [aws_security_group.ecs_sg.id]
  }
}

# 3. RDS PostgreSQL (Authz)
resource "aws_db_instance" "postgres" {
  identifier           = "pulse-authz-db"
  allocated_storage    = 20
  engine               = "postgres"
  engine_version       = "15.3"
  instance_class       = "db.t3.micro"
  username             = var.db_username
  password             = var.db_password
  parameter_group_name = "default.postgres15"
  skip_final_snapshot  = true
  vpc_security_group_ids = [aws_security_group.db_cache_sg.id]
  db_subnet_group_name   = module.vpc.database_subnet_group_name
}

# 4. ElastiCache Valkey/Redis (Signaling)
resource "aws_elasticache_cluster" "valkey" {
  cluster_id           = "pulse-signaling-cache"
  engine               = "redis"
  node_type            = "cache.t3.micro"
  num_cache_nodes      = 1
  parameter_group_name = "default.redis7"
  engine_version       = "7.1"
  port                 = 6379
  security_group_ids   = [aws_security_group.db_cache_sg.id]
  subnet_group_name    = module.vpc.elasticache_subnet_group_name
}

# 5. ECS Fargate Cluster (pulse-cloud)
resource "aws_ecs_cluster" "main" {
  name = "pulse-cloud-cluster"
}

resource "aws_ecs_task_definition" "pulse_cloud" {
  family                   = "pulse-cloud-task"
  network_mode             = "awsvpc"
  requires_compatibilities = ["FARGATE"]
  cpu                      = 256
  memory                   = 512

  container_definitions = jsonencode([
    {
      name      = "pulse-cloud"
      image     = "${var.ecr_repository_url}:latest"
      essential = true
      portMappings = [
        {
          containerPort = 9091
          hostPort      = 9091
        }
      ]
      environment = [
        { name = "DB_URL", value = "postgres://${var.db_username}:${var.db_password}@${aws_db_instance.postgres.endpoint}/${var.db_name}" },
        { name = "VALKEY_URL", value = aws_elasticache_cluster.valkey.cache_nodes[0].address }
      ]
    }
  ])
}

resource "aws_ecs_service" "pulse_cloud_svc" {
  name            = "pulse-cloud-svc"
  cluster         = aws_ecs_cluster.main.id
  task_definition = aws_ecs_task_definition.pulse_cloud.arn
  desired_count   = 3 # Horizontal Scaling
  launch_type     = "FARGATE"

  network_configuration {
    subnets         = module.vpc.private_subnets
    security_groups = [aws_security_group.ecs_sg.id]
  }

  load_balancer {
    target_group_arn = aws_lb_target_group.pulse_tg.arn
    container_name   = "pulse-cloud"
    container_port   = 9091
  }
}

# 6. Application Load Balancer
resource "aws_lb" "main" {
  name               = "pulse-alb"
  internal           = false
  load_balancer_type = "application"
  security_groups    = [aws_security_group.alb_sg.id]
  subnets            = module.vpc.public_subnets
}

resource "aws_lb_target_group" "pulse_tg" {
  name        = "pulse-cloud-tg"
  port        = 9091
  protocol    = "HTTP" # WSS upgrades from HTTP
  vpc_id      = module.vpc.vpc_id
  target_type = "ip"
  
  health_check {
    path = "/health"
  }
}

resource "aws_lb_listener" "https" {
  load_balancer_arn = aws_lb.main.arn
  port              = "443"
  protocol          = "HTTPS"
  ssl_policy        = "ELBSecurityPolicy-2016-08"
  certificate_arn   = var.acm_certificate_arn

  default_action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.pulse_tg.arn
  }
}
