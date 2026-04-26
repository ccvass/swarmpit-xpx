# Configuration

Swarmpit XPX behavior is configured via environment variables.

## Core

| Variable | Default | Description |
|---|---|---|
| `SWARMPIT_DB_PATH` | `./data` | SQLite database directory |
| `SWARMPIT_PUBLIC_DIR` | `resources/public` | Frontend static files directory |
| `SWARMPIT_INSTANCE_NAME` | (none) | Custom name shown in sidebar and browser tab |
| `PORT` | `8080` | HTTP listen port |

## Auto-Setup

| Variable | Default | Description |
|---|---|---|
| `SWARMPIT_ADMIN_USER` | (none) | Auto-create admin user on first start |
| `SWARMPIT_ADMIN_PASSWORD` | (none) | Password for auto-created admin |

## TLS

| Variable | Default | Description |
|---|---|---|
| `SWARMPIT_TLS_CERT` | (none) | Path to TLS certificate (enables HTTPS) |
| `SWARMPIT_TLS_KEY` | (none) | Path to TLS private key |

## Security

| Variable | Default | Description |
|---|---|---|
| `SWARMPIT_WEBHOOK_SECRET` | (none) | HMAC secret for GitOps webhook validation |

## OAuth2/SSO

Set per provider (replace `{PROVIDER}` with `GITHUB`, `GITLAB`, `GOOGLE`, etc.):

| Variable | Description |
|---|---|
| `OAUTH_{PROVIDER}_CLIENT_ID` | OAuth2 client ID |
| `OAUTH_{PROVIDER}_CLIENT_SECRET` | OAuth2 client secret |
| `OAUTH_{PROVIDER}_AUTH_URL` | Authorization endpoint |
| `OAUTH_{PROVIDER}_TOKEN_URL` | Token endpoint |
| `OAUTH_{PROVIDER}_USERINFO_URL` | User info endpoint |
| `OAUTH_{PROVIDER}_REDIRECT_URI` | Callback URL |
| `OAUTH_DEFAULT_ROLE` | Default role for new OAuth users (default: `user`) |

## Agent Configuration

The Swarmpit agent pushes node/task stats. Set these on the agent service:

| Variable | Description |
|---|---|
| `EVENT_ENDPOINT` | URL for event push (e.g., `http://tools_swarmpit:8080/events`) |
| `HEALTH_CHECK_ENDPOINT` | URL for health check (e.g., `http://tools_swarmpit:8080/version`) |
| `DOCKER_API_VERSION` | Docker API version (e.g., `1.35`) |

## API Endpoints

### Public (no auth)

| Endpoint | Method | Description |
|---|---|---|
| `/health/live` | GET | Liveness probe |
| `/health/ready` | GET | Readiness probe (Docker + SQLite) |
| `/version` | GET | App version and Docker info |
| `/api-docs` | GET | Swagger UI |
| `/login` | POST | Basic auth login, returns JWT |
| `/initialize` | POST | Create first admin user |
| `/oauth/{provider}/login` | GET | OAuth2 redirect to provider |
| `/oauth/{provider}/callback` | GET | OAuth2 callback, returns JWT |
| `/api/webhooks/{token}` | POST | Trigger service redeploy (token-based) |
| `/api/webhooks/git/{id}` | POST | GitOps webhook trigger |

### Authenticated (JWT required)

| Endpoint | Method | Role | Description |
|---|---|---|---|
| `/api/services` | GET | any | List services |
| `/api/services` | POST | user+ | Create service |
| `/api/services/{id}` | GET | any | Service detail |
| `/api/services/{id}/update` | POST | user+ | Update service |
| `/api/services/{id}/redeploy` | POST | user+ | Force redeploy |
| `/api/services/{id}/rollback` | POST | user+ | Rollback to previous |
| `/api/services/{id}/stop` | POST | user+ | Scale to 0 |
| `/api/services/{id}/logs` | GET | any | Service logs (add `?follow=true` for SSE) |
| `/api/services/{id}/compose` | GET | any | Service compose YAML |
| `/api/stacks` | GET | any | List stacks (with timestamps) |
| `/api/stacks` | POST | user+ | Deploy stack from compose |
| `/api/stacks/import` | POST | admin | Bulk import stacks |
| `/api/stacks/{name}/compose` | GET | any | Stack compose YAML (full round-trip) |
| `/api/nodes` | GET | any | List nodes |
| `/api/gitops` | GET/POST | any/user+ | GitOps stack management |
| `/api/registry/{type}` | GET/POST | any/user+ | Registry management |
| `/api/tags/*` | GET | any | Image tags (DockerHub, GHCR, GitLab, ECR) |
| `/api/totp/setup` | POST | any | Enable 2FA for current user |
| `/api/totp/disable` | POST | any | Disable 2FA |
| `/events` | GET | any | SSE real-time updates |
| `/events` | POST | any | Agent stats push |
| `/exec/{id}` | GET | any | WebSocket container shell |

### Admin only

| Endpoint | Method | Description |
|---|---|---|
| `/api/users` | GET/POST | User management |
| `/api/audit` | GET | Audit log |
| `/api/backup` | GET | Export all data |
| `/api/restore` | POST | Import data |
| `/api/teams` | GET/POST | Team permissions (RBAC) |
| `/api/clusters` | GET/POST | Multi-cluster management |
| `/api/clusters/{id}/activate` | POST | Switch active cluster |
| `/api/system/prune` | POST | Clean unused resources |
| `/api/services/check-updates` | POST | Check for image updates |

## CLI Commands

```bash
# Run the server
./swarmpit

# Reset a user's password
./swarmpit reset-password <username> [new-password]
# If new-password is omitted, defaults to "changeme"
```
