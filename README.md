# Swarmpit XPX

<p align="center">
  <img src="resources/public/img/logo-xpx.svg" alt="Swarmpit XPX" width="400">
</p>

<p align="center">
  Hardened, self-contained Docker Swarm management UI. Zero external databases. Two containers. Done.
</p>

[![Test, Build & Deploy](https://github.com/ccvass/swarmpit-xpx/actions/workflows/build.yml/badge.svg)](https://github.com/ccvass/swarmpit-xpx/actions/workflows/build.yml)
[![License](https://img.shields.io/badge/license-EPL--1.0-blue.svg)](LICENSE)

## What is this

Swarmpit XPX is a security-hardened fork of [swarmpit/swarmpit](https://github.com/swarmpit/swarmpit) that eliminates external database dependencies and modernizes the runtime stack. It provides a complete Docker Swarm management UI — services, stacks, secrets, volumes, networks, registries — with real-time monitoring and a REST API.

## Why this fork

The original Swarmpit requires 4 containers (app + CouchDB + InfluxDB + agent), runs as root, uses outdated dependencies with known CVEs, and has no rate limiting or security hardening.

Swarmpit XPX fixes all of that:

| Feature | Original | XPX |
|---------|----------|-----|
| Containers | 4 | **2** (app + agent) |
| Database | CouchDB 2.3 (external, EOL) | **SQLite** (embedded) |
| Time-series | InfluxDB 1.8 (external, maintenance) | **In-memory** ring buffer |
| Runtime | Java 11, Clojure 1.10 | **Java 21**, **Clojure 1.12** |
| Concurrency | OS threads (limited pool) | **Virtual threads** (unlimited) |
| Container user | root | root (Docker socket requires it) |
| Auth protection | None | **Rate limiting** (5/min/IP) |
| Error handling | Leaks stack traces | **Generic errors** (logged server-side) |
| Health checks | Basic HTTP | **Liveness + Readiness** probes |
| Resilience | None | **Circuit breakers** per dependency |
| CORS | Wildcard `*` on SSE | **Same-origin only** |
| Backup | 2 separate volumes | **Single file** (`swarmpit.db`) |
| Docker socket | JNR Unix sockets (native lib) | **Java 21 NIO** (zero native deps) |
| Dependencies | ring 1.8, reitit 0.3 | **ring 1.12, reitit 0.7** |
| RAM usage | ~1.8 GB | **~1.0 GB** |

## Installation

The only requirement is Docker with Swarm initialized.

```bash
# Create data directory on the host
sudo mkdir -p /mnt/swarmpit-xpx

# Deploy
git clone https://github.com/ccvass/swarmpit-xpx.git
docker stack deploy -c swarmpit-xpx/docker-compose.yml swarmpit
```

Swarmpit XPX is available on port **888** by default.

> **Important**: The bind mount `/mnt/swarmpit-xpx:/app/data` persists the SQLite database.
> Without it, all data (users, registries, webhooks) is lost on container restart.

For ARM clusters (Raspberry Pi, etc.):

```bash
docker stack deploy -c swarmpit-xpx/docker-compose.arm.yml swarmpit
```

## Stack composition

Only 2 services:

- **app** — Swarmpit application (Clojure JVM + embedded SQLite)
- **agent** — Stats collector (deployed globally on every node)

Data persists in a single Docker volume (`app-data`) containing the SQLite database.

## Backup and restore

```bash
# Backup (direct from host)
cp /mnt/swarmpit-xpx/swarmpit.db ./backup.db

# Restore
cp ./backup.db /mnt/swarmpit-xpx/swarmpit.db
docker service update --force swarmpit_app
```

## Health endpoints

- `GET /health/live` — Is the JVM alive? (for orchestrator liveness probes)
- `GET /health/ready` — Are Docker socket and SQLite reachable? (for readiness probes)
- `GET /version` — Application version and Docker API info

## API and MCP

Everything the UI does is exposed via REST API (Swagger docs at `/api-docs`).

### Webhook Auto-Update

Trigger service redeploy from CI/CD pipelines without authentication:

```bash
# Create webhook for a service
curl -X POST https://swarmpit/api/webhooks -H "Authorization: $TOKEN" \
  -d '{"service-id": "abc123"}'
# Returns: {"token": "uuid-token", "service-id": "abc123"}

# Trigger redeploy (no auth needed — token IS the auth)
curl -X POST https://swarmpit/api/webhooks/<token>
```

### Git-Based Stack Deploy

Deploy stacks directly from a git repository:

```bash
curl -X POST https://swarmpit/api/stacks/git -H "Authorization: $TOKEN" \
  -d '{"repo-url": "https://github.com/org/repo", "branch": "main", "stack-name": "myapp"}'
```

### Audit Log

All mutating actions are logged. View audit trail (admin only):

```bash
curl https://swarmpit/api/audit -H "Authorization: $TOKEN"
```

### Container Exec

WebSocket endpoint for interactive shell access:

```
ws://swarmpit/exec/<container-id>
```

For LLM-driven workflows, use the [MCP server](https://github.com/swarmpit/mcp) — manage your Swarm from Claude Code, Kiro, or any MCP-compatible client.

## Environment variables

Refer to [docs/CONFIGURATION.md](docs/CONFIGURATION.md) for the full list.

## Development

Swarmpit XPX is written in Clojure (backend) and ClojureScript (frontend, Rum/React).

```bash
# Prerequisites: Leiningen, Java 21+, Docker socket at /var/run/docker.sock

lein deps                    # Install dependencies
lein repl                    # Start REPL, then (fig-start) for dev server
lein test :all               # Run all tests
lein with-profile prod uberjar  # Build production JAR
docker build -t swarmpit-xpx .  # Build Docker image
```

## Upstream

This project is a fork of [swarmpit/swarmpit](https://github.com/swarmpit/swarmpit). We maintain compatibility with the upstream API and agent protocol while diverging on architecture and security.

## License

[Eclipse Public License 1.0](LICENSE)
