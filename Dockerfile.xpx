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
ARG VERSION=dev
RUN CGO_ENABLED=1 go build -ldflags="-s -w -X main.version=${VERSION}" -o swarmpit ./cmd/swarmpit

# Stage 3: Runtime
FROM alpine:3.21
LABEL org.opencontainers.image.source="https://github.com/ccvass/swarmpit-xpx"
RUN apk add --no-cache docker-cli curl ca-certificates git && mkdir -p /app/data
WORKDIR /app
COPY --from=builder /app/swarmpit .
COPY --from=frontend /app/resources/public/ /app/public/
COPY resources/index.html /app/public/
COPY resources/public/js/xpx-features.js /app/public/js/xpx-features.js

# Rebrand: replace ALL purple/lilac with blue in compiled JS and CSS
RUN sed -i \
    -e 's/#65519f/#1565C0/g' \
    -e 's/#7e57c2/#1E88E5/g' \
    -e 's/#957ed1/#42A5F5/g' \
    -e 's/#7564CC/#42A5F5/g' \
    -e 's/#8543E0/#1E88E5/g' \
    -e 's/#A877ED/#64B5F6/g' \
    -e 's/#8884d8/#42A5F5/g' \
    -e 's/#D598D9/#90CAF9/g' \
    -e 's/#DD81E6/#64B5F6/g' \
    -e 's/#311B92/#0D47A1/g' \
    -e 's/#4A148C/#0D47A1/g' \
    -e 's/#4527A0/#0D47A1/g' \
    -e 's/#512da8/#0D47A1/g' \
    -e 's/#5e35b1/#1565C0/g' \
    -e 's/#673ab7/#1976D2/g' \
    -e 's/#7B1FA2/#1565C0/g' \
    -e 's/#7b1fa2/#1565C0/g' \
    -e 's/#8e24aa/#1976D2/g' \
    -e 's/#9C27B0/#1976D2/g' \
    -e 's/#9c27b0/#1976D2/g' \
    -e 's/#AB47BC/#42A5F5/g' \
    -e 's/#ab47bc/#42A5F5/g' \
    -e 's/#BA68C8/#64B5F6/g' \
    -e 's/#ba68c8/#64B5F6/g' \
    -e 's/#CE93D8/#90CAF9/g' \
    -e 's/#ce93d8/#90CAF9/g' \
    -e 's/#E1BEE7/#BBDEFB/g' \
    -e 's/#e1bee7/#BBDEFB/g' \
    -e 's/#F3E5F5/#E3F2FD/g' \
    -e 's/#f3e5f5/#E3F2FD/g' \
    -e 's/#EDE7F6/#E3F2FD/g' \
    -e 's/#ede7f6/#E3F2FD/g' \
    -e 's/#D1C4E9/#BBDEFB/g' \
    -e 's/#d1c4e9/#BBDEFB/g' \
    -e 's/#B39DDB/#90CAF9/g' \
    -e 's/#b39ddb/#90CAF9/g' \
    -e 's/#9575CD/#64B5F6/g' \
    -e 's/#9575cd/#64B5F6/g' \
    -e 's/#6A1B9A/#0D47A1/g' \
    -e 's/#6a1b9a/#0D47A1/g' \
    -e 's/#9467bd/#42A5F5/g' \
    -e 's/#6200ea/#1565C0/g' \
    -e 's/#651fff/#1E88E5/g' \
    -e 's/#aa00ff/#1565C0/g' \
    -e 's/#d500f9/#1E88E5/g' \
    -e 's/#e040fb/#42A5F5/g' \
    -e 's/#ea80fc/#90CAF9/g' \
    -e 's/#b388ff/#90CAF9/g' \
    -e 's/#7c4dff/#1E88E5/g' \
    -e 's/#e8eaf6/#E3F2FD/g' \
    -e 's/#c5cae9/#BBDEFB/g' \
    -e 's/#9fa8da/#90CAF9/g' \
    -e 's/#7986cb/#64B5F6/g' \
    -e 's/#5c6bc0/#42A5F5/g' \
    -e 's/#536dfe/#1E88E5/g' \
    -e 's/#8c9eff/#90CAF9/g' \
    -e 's/#8082FF/#64B5F6/g' \
    -e 's/rgb(126,87,194)/rgb(30,136,229)/g' \
    -e 's/rgb(103,58,183)/rgb(25,118,210)/g' \
    -e 's/rgb(101,81,159)/rgb(21,101,192)/g' \
    -e 's/rgb(156,39,176)/rgb(25,118,210)/g' \
    -e 's/rgb(123,31,162)/rgb(21,101,192)/g' \
    -e 's/rgb(149,126,209)/rgb(66,165,245)/g' \
    -e 's/rgb(133,67,224)/rgb(30,136,229)/g' \
    -e 's/rgb(117,100,204)/rgb(66,165,245)/g' \
    /app/public/js/main.js /app/public/css/main.css

# Replace logo and icon
COPY resources/public/img/logo.svg /app/public/img/logo.svg
COPY resources/public/img/icon.svg /app/public/img/icon.svg

# Cache-bust: rename logo/icon with version hash and update references in JS
RUN HASH=$(md5sum /app/public/img/logo.svg | cut -c1-8) && \
    cp /app/public/img/logo.svg /app/public/img/logo.${HASH}.svg && \
    cp /app/public/img/icon.svg /app/public/img/icon.${HASH}.svg && \
    sed -i "s|img/logo\.svg|img/logo.${HASH}.svg|g" /app/public/js/main.js /app/public/index.html && \
    sed -i "s|img/icon\.svg|img/icon.${HASH}.svg|g" /app/public/js/main.js /app/public/index.html

# Replace branding text in compiled JS
RUN sed -i \
    -e 's/swarmpit\.io/swarmpit-xpx/g' \
    -e 's/team@swarmpit/team@swarmpit-xpx/g' \
    -e 's/github\.com\/swarmpit\/swarmpit/github.com\/ccvass\/swarmpit-xpx/g' \
    -e 's/\[\"swarmpit \"/[\"swarmpit-xpx \"/g' \
    -e 's/\" :: swarmpit\"/" :: swarmpit-xpx"/g' \
    -e 's/"network-autocomplete",oz,!0,Sr,!0,Nv,a,cI,b/"network-autocomplete",oz,!0,Sr,!0,Nv,a||[],cI,b||[]/g' \
    -e 's/"placement-autocomplete",oz,!0,Sr,!0,cI,b,Nv,a/"placement-autocomplete",oz,!0,Sr,!0,cI,b||[],Nv,a||[]/g' \
    /app/public/js/main.js

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
  CMD curl -fs http://localhost:8080/health/live || exit 1

ENV SWARMPIT_PUBLIC_DIR=/app/public
ENV SWARMPIT_DB_PATH=/app/data
# NOTE: runs as root because Docker socket access requires it in Swarm mode.
# The container is hardened via read-only filesystem and no-new-privileges in the stack deploy.
EXPOSE 8080
CMD ["./swarmpit"]
