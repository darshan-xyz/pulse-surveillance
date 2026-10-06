variable "aws_region" {
  description = "AWS Region to deploy the Pulse stack"
  type        = string
  default     = "us-east-1"
}

variable "db_username" {
  description = "PostgreSQL Admin Username"
  type        = string
  sensitive   = true
}

variable "db_password" {
  description = "PostgreSQL Admin Password"
  type        = string
  sensitive   = true
}

variable "db_name" {
  description = "Database name"
  type        = string
  default     = "pulse_authz"
}

variable "ecr_repository_url" {
  description = "URL of the ECR repository holding the pulse-cloud Docker image"
  type        = string
}

variable "acm_certificate_arn" {
  description = "ARN of the ACM certificate for HTTPS/WSS termination on the ALB"
  type        = string
}
