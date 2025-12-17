FROM golang:1.21-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o url-service ./services/url-service

FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /root/

COPY --from=builder /app/url-service .
COPY services/url-service/migrations /migrations

RUN wget -O /usr/local/bin/migrate https://github.com/golang-migrate/migrate/releases/download/v4.16.2/migrate.linux-amd64.tar.gz && \
  tar -xzf /usr/local/bin/migrate && \
  chmod +x /usr/local/bin/migrate

EXPOSE 50051

CMD ["./url-service"]
