# Swarmpit XPX

Lightweight Docker Swarm management UI — rewritten in Go.

A fork of [Swarmpit](https://github.com/swarmpit/swarmpit) with a Go backend replacing the original Clojure/JVM stack. Same ClojureScript frontend, 10x lighter runtime.

## Why XPX?

| | Swarmpit (original) | Swarmpit XPX |
|---|---|---|
| Backend | Clojure/JVM (93MB JAR + 300MB JVM) | Go (11MB binary) |
| Startup | 5-10 seconds | < 1 second |
| RAM usage | ~300MB | ~30MB |
| Database | CouchDB (external) + InfluxDB (external) | SQLite (embedded) |
| Dependencies | 3 containers (app + CouchDB + InfluxDB) | 1 container (app only) |

## Features

### Core (Swarmpit parity)

- Dashboard with CPU/RAM/disk gauges and timeseries charts
- Services — list, detail, create, edit, stop, redeploy, rollback
- Stacks — list, detail, deploy from compose, redeploy, rollback, activate/deactivate
- Nodes — list, detail with stats, edit labels/availability
- Networks, Volumes, Secrets, Configs — full CRUD
- Tasks — list, detail with per-task stats
- Real-time updates via SSE subscription routing
- Registry management (DockerHub, v2, ECR, ACR, GitLab)
- User management with JWT auth
- Audit log

### New in XPX

- **Alerting** — CPU/RAM/disk threshold alerts via webhook
- **Service templates** — save and reuse service configurations
- **Auto-deploy** — webhook triggers redeploy on image push (DockerHub/GHCR)
- **Streaming logs** — real-time log streaming via SSE
- **Swagger/OpenAPI** — API documentation at `/api-docs`
- **Compose validation** — validate YAML before deploying
- **Health check status** — per-service health aggregation
- **Backup/restore** — export/import all configuration
- **Dashboard pinning** — pin favorite services and nodes
- **Per-service timeseries** — CPU/RAM charts per service
- **Multi-arch** — AMD64 + ARM64 Docker images

## Quick Start

```bash
docker stack deploy -c docker-compose.yml swarmpit
```

Or with Docker service:

```bash
docker service create \
  --name swarmpit \
  --publish 888:8080 \
  --constraint 'node.role == manager' \
  --mount type=bind,src=/var/run/docker.sock,dst=/var/run/docker.sock \
  --mount type=bind,src=/mnt/swarmpit,dst=/app/data \
  ghcr.io/ccvass/swarmpit-xpx:latest
```

## Configuration

| Environment Variable | Default | Description |
|---|---|---|
| `SWARMPIT_DB_PATH` | `/app/data` | SQLite database directory |
| `SWARMPIT_PUBLIC_DIR` | `/app/public` | Frontend static files |
| `SWARMPIT_INSTANCE_NAME` | (none) | Custom name shown in sidebar |

## API Documentation

Available at `/api-docs` when the service is running.

## Development

```bash
# Build Go backend
CGO_ENABLED=1 go build -o swarmpit ./cmd/swarmpit

# Build Docker image (includes ClojureScript frontend)
docker build -f Dockerfile.go -t swarmpit-xpx .

# Run locally
./swarmpit
```

## License

GNU Affero General Public License v3.0
