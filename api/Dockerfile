# Build
FROM golang:1.26-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 go build \
    -trimpath \
    -ldflags="-s -w" \
    -o api ./cmd

# Runtime
FROM alpine:3.22

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /

COPY --from=builder /app/api /api

EXPOSE 8000 8080 9090

ENTRYPOINT ["/api"]