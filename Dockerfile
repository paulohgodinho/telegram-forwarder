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

FROM alpine:3.20
WORKDIR /app

# Provide the standard CA bundle and DNS/runtime files needed for Docker names like "n8n" and outbound HTTPS.
RUN apk add --no-cache ca-certificates

# Copy the compiled binary into the mounted app directory
COPY --from=builder /out/telegram-forwarder /app/telegram-forwarder

ENTRYPOINT ["/app/telegram-forwarder"]
