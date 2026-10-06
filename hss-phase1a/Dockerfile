FROM golang:1.26.5-alpine AS builder

WORKDIR /app

RUN apk add --no-cache gcc musl-dev

# Copy go.mod and go.sum
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the code
COPY . .

# Build the HSS daemon. We enable CGO for SQLite.
RUN go mod tidy
RUN CGO_ENABLED=1 GOOS=linux go build -o /app/bin/hss ./cmd/hss/main.go

FROM alpine:latest

WORKDIR /app

# Install dependencies required by SQLite/CGO in alpine runtime
RUN apk add --no-cache ca-certificates tzdata sqlite-libs

COPY --from=builder /app/bin/hss /app/hss
COPY --from=builder /app/config.yaml /app/config.yaml

# Data directory for SQLite and Recordings
VOLUME ["/app/data", "/app/recordings"]

EXPOSE 8080

CMD ["/app/hss"]
