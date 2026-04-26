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
- Stacks — list with timestamps, detail, deploy from compose, redeploy, rollback, activate/deactivate
- Nodes — list, detail with stats, edit labels/availability
- Networks, Volumes, Secrets, Configs — full CRUD
- Tasks — list, detail with per-task stats
- Real-time updates via SSE subscription routing (authenticated)
- Registry management (DockerHub, GHCR, GitLab, ECR, generic v2)
- User management with JWT auth and role-based access control
- Audit log

### New in XPX

- **Full compose round-trip** — stack compose generation preserves all fields: entrypoint, healthcheck, deploy config, resources, placement, update/rollback config, labels, network aliases, configs, secrets, sysctls, cap_add/drop, logging
- **Registry auth** — `--with-registry-auth` on all stack deploys; token exchange for DockerHub, GitLab, ECR, GHCR
- **OAuth2/SSO** — login via any OIDC provider (GitHub, GitLab, Google, Keycloak, etc.)
- **2FA/TOTP** — optional two-factor authentication per user
- **RBAC** — viewer/user/admin roles; viewers blocked from write operations; granular team-based stack permissions
- **Multi-cluster** — register and switch between multiple Docker Swarm clusters
- **GitOps** — deploy and auto-sync stacks from Git repositories with webhook support
- **Alerting** — CPU/RAM/disk threshold alerts via webhook
- **Service templates** — save and reuse service configurations
- **Auto-deploy** — webhook triggers redeploy on image push (DockerHub/GHCR)
- **Streaming logs** — real-time log streaming via SSE with ANSI color rendering
- **Bulk stack import** — deploy multiple stacks in a single API call
- **Backup/restore** — export/import all configuration with credential preservation
- **Built-in TLS** — optional HTTPS via environment variables
- **Password reset CLI** — `./swarmpit reset-password <username>` command
- **Swagger/OpenAPI** — API documentation at `/api-docs`
- **Compose validation** — validate YAML before deploying
- **Health check status** — per-service health aggregation
- **Dashboard pinning** — pin favorite services and nodes
- **Per-service timeseries** — CPU/RAM charts per service
- **Container scanning** — Trivy security scanning in CI/CD
- **Multi-arch** — AMD64 + ARM64 Docker images

## Quick Start

```bash
docker service create \
  --name swarmpit \
  --publish 888:8080 \
  --constraint 'node.role == manager' \
  --mount type=bind,src=/var/run/docker.sock,dst=/var/run/docker.sock \
  --mount type=bind,src=/mnt/swarmpit,dst=/app/data \
  ghcr.io/ccvass/swarmpit-xpx:go
```

## Configuration

| Environment Variable | Default | Description |
|---|---|---|
| `SWARMPIT_DB_PATH` | `/app/data` | SQLite database directory |
| `SWARMPIT_PUBLIC_DIR` | `/app/public` | Frontend static files |
| `SWARMPIT_INSTANCE_NAME` | (none) | Custom name shown in sidebar |
| `SWARMPIT_ADMIN_USER` | (none) | Auto-create admin on first start |
| `SWARMPIT_ADMIN_PASSWORD` | (none) | Admin password for auto-creation |
| `SWARMPIT_TLS_CERT` | (none) | TLS certificate path (enables HTTPS) |
| `SWARMPIT_TLS_KEY` | (none) | TLS private key path |
| `SWARMPIT_WEBHOOK_SECRET` | (none) | HMAC secret for GitOps webhook validation |
| `PORT` | `8080` | HTTP listen port |

### OAuth2 Configuration

Set per provider (replace `{PROVIDER}` with `GITHUB`, `GITLAB`, `GOOGLE`, etc.):

| Variable | Description |
|---|---|
| `OAUTH_{PROVIDER}_CLIENT_ID` | OAuth2 client ID |
| `OAUTH_{PROVIDER}_CLIENT_SECRET` | OAuth2 client secret |
| `OAUTH_{PROVIDER}_AUTH_URL` | Authorization endpoint |
| `OAUTH_{PROVIDER}_TOKEN_URL` | Token endpoint |
| `OAUTH_{PROVIDER}_USERINFO_URL` | User info endpoint |
| `OAUTH_{PROVIDER}_REDIRECT_URI` | Callback URL |
| `OAUTH_DEFAULT_ROLE` | Default role for OAuth users (`user`) |

## API Documentation

Available at `/api-docs` when the service is running.

## CLI Commands

```bash
# Run the server
./swarmpit

# Reset a user's password
./swarmpit reset-password <username> [new-password]
```

## Development

```bash
# Build
CGO_ENABLED=1 go build -ldflags="-s -w -X main.version=dev" -o swarmpit ./cmd/swarmpit

# Test
go test ./... -race -cover

# Lint
go vet ./...

# Docker image
docker build -f Dockerfile.xpx -t swarmpit-xpx .
```

## Test Coverage

| Package | Coverage |
|---|---|
| `internal/store` | 91.8% |
| `internal/auth` | 100% |
| `internal/api` | Pure logic tests (Docker-dependent handlers require live cluster) |

## License

GNU Affero General Public License v3.0
