# Configuration

Swarmpit XPX behavior can be reconfigured via environment variables.

## `SWARMPIT_DB_PATH`

Directory where the embedded SQLite database is stored.
Default is `./data` (relative to working directory). Set to `/data` in Docker.

## `SWARMPIT_DOCKER_SOCK`

Docker socket location used by the Docker client. For TCP proxy use: `http://host:2375`.
Default is `/var/run/docker.sock`.

## `SWARMPIT_DOCKER_API`

Docker API version. Auto-negotiated via `/_ping` at startup. Override only if needed.
Default is auto-negotiated (max `1.44`).

## `SWARMPIT_DOCKER_HTTP_TIMEOUT`

Docker client HTTP timeout in milliseconds.
Default is `5000`.

## `SWARMPIT_LOG_LEVEL`

Application log level: `debug`, `info`, `warn`, `error`.
Default is `info`.

## `SWARMPIT_INSTANCE_NAME`

Custom name shown in place of the swarmpit logo in the sidebar and top bar, and prepended to the browser tab title. Useful when running multiple instances against different clusters.
Default is `nil` (shows the swarmpit logo).

## `SWARMPIT_AGENT_URL`

Static agent address. If `nil`, value is discovered dynamically. For development only.
Default is `nil`.

## Agent Configuration

The Swarmpit agent needs to know where to send events. When deploying as a named stack (e.g., `tools`), set these environment variables on the agent service:

- `EVENT_ENDPOINT` — URL for event push (e.g., `http://tools_swarmpit:8080/events`)
- `HEALTH_CHECK_ENDPOINT` — URL for health check (e.g., `http://tools_swarmpit:8080/version`)

The hostname must match the service name in the Docker overlay network.

## API Endpoints

| Endpoint | Method | Auth | Description |
|----------|--------|------|-------------|
| `/health/live` | GET | No | Liveness probe |
| `/health/ready` | GET | No | Readiness probe (checks Docker + SQLite) |
| `/version` | GET | No | App version and Docker API info |
| `/api/webhooks/:token` | POST | No (token-based) | Trigger service redeploy |
| `/api/webhooks` | POST | User | Create webhook for a service |
| `/api/audit` | GET | Admin | View audit log |
| `/api/stacks/git` | POST | User | Deploy stack from git repo |
| `/exec/:id` | GET (WebSocket) | User | Interactive container shell |
| `/api/services` | GET | User | List services |
| `/api/nodes` | GET | User | List nodes |
| `/api-docs` | GET | No | Swagger UI |
