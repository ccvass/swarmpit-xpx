# Stage 1: Build frontend (ClojureScript)
FROM eclipse-temurin:21-jdk-jammy AS frontend
WORKDIR /app
RUN apt-get update && apt-get install -y --no-install-recommends curl && rm -rf /var/lib/apt/lists/* && \
    curl -fsSL https://raw.githubusercontent.com/technomancy/leiningen/stable/bin/lein -o /usr/local/bin/lein && \
    chmod +x /usr/local/bin/lein && lein version
COPY project.clj ./
COPY repo/ repo/
RUN lein deps
COPY src/ src/
COPY resources/ resources/
RUN lein cljsbuild once min

# Stage 2: Build Go backend
FROM golang:1.26-alpine AS builder
RUN apk add --no-cache gcc musl-dev git
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ cmd/
COPY internal/ internal/
RUN CGO_ENABLED=1 go build -ldflags="-s -w" -o swarmpit ./cmd/swarmpit

# Stage 3: Runtime
FROM alpine:3.21

LABEL org.opencontainers.image.source="https://github.com/ccvass/swarmpit-xpx"

RUN apk add --no-cache docker-cli curl ca-certificates && mkdir -p /app/data

WORKDIR /app
COPY --from=builder /app/swarmpit .
COPY --from=frontend /app/resources/public/ /app/public/
COPY resources/index.html /app/public/

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
  CMD curl -fs http://localhost:8080/health/live || exit 1

ENV SWARMPIT_PUBLIC_DIR=/app/public
ENV SWARMPIT_DB_PATH=/app/data

EXPOSE 8080
CMD ["./swarmpit"]
