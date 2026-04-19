FROM eclipse-temurin:21-jre-jammy

RUN apt-get update && \
    apt-get install -y --no-install-recommends \
      ca-certificates \
      curl \
      tini && \
    rm -rf /var/lib/apt/lists/*

COPY --from=docker:27-cli /usr/local/bin/docker /usr/local/bin/docker

RUN groupadd -r swarmpit && \
    useradd -r -g swarmpit -d /usr/src/app -s /sbin/nologin swarmpit && \
    mkdir -p /usr/src/app /tmp/swarmpit && \
    chown -R swarmpit:swarmpit /usr/src/app /tmp/swarmpit

WORKDIR /usr/src/app
COPY --chown=swarmpit:swarmpit target/swarmpit.jar .

USER swarmpit

HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
  CMD curl --fail -s http://localhost:8080/version || exit 1

EXPOSE 8080
ENTRYPOINT ["tini", "--"]
CMD ["java", "-XX:+UseContainerSupport", "-XX:MaxRAMPercentage=75.0", "-jar", "swarmpit.jar"]
