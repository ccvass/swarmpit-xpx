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
