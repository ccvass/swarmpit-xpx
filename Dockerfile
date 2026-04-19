# Stage 1: Build (used for local builds; CI skips via --target=runtime)
FROM eclipse-temurin:21.0.6_7-jdk-jammy AS builder
WORKDIR /build
RUN apt-get update && \
    apt-get install -y --no-install-recommends curl && \
    rm -rf /var/lib/apt/lists/* && \
    curl -fsSL https://raw.githubusercontent.com/technomancy/leiningen/stable/bin/lein -o /usr/local/bin/lein && \
    chmod +x /usr/local/bin/lein && \
    lein version
COPY project.clj ./
RUN lein deps
COPY src/ src/
COPY resources/ resources/
COPY repo/ repo/
RUN lein with-profile prod uberjar

# Stage 2: Runtime
FROM eclipse-temurin:21.0.6_7-jre-jammy AS runtime

LABEL org.opencontainers.image.source="https://github.com/ccvass/swarmpit-xpx"
LABEL org.opencontainers.image.description="Hardened Docker Swarm management UI"

COPY --from=docker:27.5-cli /usr/local/bin/docker /usr/local/bin/docker

RUN groupadd -r swarmpit && \
    useradd -r -g swarmpit -d /app -s /sbin/nologin swarmpit && \
    mkdir -p /app /data /tmp/swarmpit && \
    chown -R swarmpit:swarmpit /app /data /tmp/swarmpit

WORKDIR /app

# CI: jar at target/swarmpit.jar (from artifact download)
# Local: jar from builder stage
COPY --chown=swarmpit:swarmpit target/swarmpit.jar .

USER swarmpit

HEALTHCHECK --interval=30s --timeout=3s --start-period=15s --retries=3 \
  CMD java -cp swarmpit.jar clojure.main -e '(System/exit 0)' || exit 1

EXPOSE 8080
ENTRYPOINT ["java", "-XX:+UseContainerSupport", "-XX:MaxRAMPercentage=75.0", \
            "-XX:+UseZGC", "-XX:+ZGenerational", \
            "-Djava.security.egd=file:/dev/./urandom"]
CMD ["-jar", "swarmpit.jar"]
