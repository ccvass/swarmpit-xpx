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

# Rebrand: replace ALL purple/lilac with blue in compiled JS and CSS
RUN sed -i \
    -e 's/#65519f/#1565C0/g' \
    -e 's/#7e57c2/#1E88E5/g' \
    -e 's/#7B1FA2/#1565C0/g' \
    -e 's/#9C27B0/#1976D2/g' \
    -e 's/#CE93D8/#90CAF9/g' \
    -e 's/#AB47BC/#42A5F5/g' \
    -e 's/#E1BEE7/#BBDEFB/g' \
    -e 's/#F3E5F5/#E3F2FD/g' \
    -e 's/#EDE7F6/#E3F2FD/g' \
    -e 's/#9575CD/#64B5F6/g' \
    -e 's/#512DA8/#0D47A1/g' \
    -e 's/#6A1B9A/#0D47A1/g' \
    -e 's/#4A148C/#0D47A1/g' \
    -e 's/#D1C4E9/#BBDEFB/g' \
    -e 's/#B39DDB/#90CAF9/g' \
    -e 's/#673AB7/#1976D2/g' \
    -e 's/#5E35B1/#1565C0/g' \
    -e 's/#4527A0/#0D47A1/g' \
    -e 's/#311B92/#0D47A1/g' \
    -e 's/rgb(126,87,194)/rgb(30,136,229)/g' \
    -e 's/rgb(103,58,183)/rgb(25,118,210)/g' \
    -e 's/rgb(101,81,159)/rgb(21,101,192)/g' \
    -e 's/rgb(156,39,176)/rgb(25,118,210)/g' \
    -e 's/rgb(123,31,162)/rgb(21,101,192)/g' \
    /app/public/js/main.js /app/public/css/main.css

# Replace logo
COPY resources/public/img/logo.svg /app/public/img/logo.svg

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
  CMD curl -fs http://localhost:8080/health/live || exit 1

ENV SWARMPIT_PUBLIC_DIR=/app/public
ENV SWARMPIT_DB_PATH=/app/data
EXPOSE 8080
CMD ["./swarmpit"]
