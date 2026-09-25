# syntax=docker/dockerfile:1

FROM golang:1.26-alpine AS builder
WORKDIR /src

# Copy dependency metadata first for better caching
COPY go.mod go.sum* ./
RUN go mod download

# Copy the rest of the source and build a static Linux binary
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags='-s -w' -o /out/telegram-forwarder .

FROM scratch
WORKDIR /app

# Include TLS root certificates for outbound HTTPS connections
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

# Copy the compiled binary into the mounted app directory
COPY --from=builder /out/telegram-forwarder /app/telegram-forwarder

ENTRYPOINT ["/app/telegram-forwarder"]
