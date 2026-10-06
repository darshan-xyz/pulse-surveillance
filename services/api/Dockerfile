FROM golang:1.26.5-alpine AS builder

WORKDIR /app

# Copy go.mod and go.sum
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the code
COPY . .

# Build the Central API daemon
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/bin/central-api ./cmd/api/main.go

FROM alpine:latest

WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /app/bin/central-api /app/central-api

EXPOSE 9090

CMD ["/app/central-api"]
