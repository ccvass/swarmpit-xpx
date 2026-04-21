FROM golang:1.26-alpine AS builder
RUN apk add --no-cache gcc musl-dev git
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ cmd/
COPY internal/ internal/
RUN CGO_ENABLED=1 go build -ldflags="-s -w" -o swarmpit ./cmd/swarmpit

FROM alpine:3.21

LABEL org.opencontainers.image.source="https://github.com/ccvass/swarmpit-xpx"
LABEL org.opencontainers.image.description="Hardened Docker Swarm management UI"

RUN apk add --no-cache docker-cli curl ca-certificates && \
    mkdir -p /app/data

WORKDIR /app
COPY --from=builder /app/swarmpit .
COPY resources/public/ /app/public/

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
  CMD curl -fs http://localhost:8080/health/live || exit 1

ENV SWARMPIT_PUBLIC_DIR=/app/public
ENV SWARMPIT_DB_PATH=/app/data

EXPOSE 8080
CMD ["./swarmpit"]
