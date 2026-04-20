# Development Standards

Project-specific development rules for Swarmpit XPX.

## Language and Runtime

- **Clojure** 1.12.0 (backend)
- **ClojureScript** 1.11.132 (frontend)
- **Java** 21 LTS (Eclipse Temurin)
- **Leiningen** 2.8.2+ (build tool)

## Code Style

- Follow standard Clojure idioms and naming conventions
- Namespaces mirror directory structure: `swarmpit.docker.engine.client` = `src/clj/swarmpit/docker/engine/client.clj`
- Prefer pure functions; side effects isolated to boundary namespaces
- Use `defn-` for private functions, `def ^:private` for private vars
- Destructuring over `get`/`get-in` when keys are known

## Formatting and Linting

- **clj-kondo** for static analysis (config in `.clj-kondo/config.edn`)
- **cljfmt** for formatting (2-space indent, align map values)
- Run `lein check` before every commit (compile sanity gate)

## Testing

- Unit tests in `test/clj/` mirroring `src/clj/` structure
- Integration tests tagged `^:integration` (require Docker)
- Run: `lein test` (unit), `lein test :integration` (integration), `lein test :all`

## Architecture Patterns

- **Embedded storage**: SQLite via next.jdbc (no external DB)
- **In-memory stats**: Ring buffers in atoms (no external time-series DB)
- **Docker socket**: Java 21 native NIO SocketChannel over Unix domain sockets (no JNR, no Apache HttpClient)
- **Aggressive caching**: Docker API reads cached 5s TTL (services, nodes, networks, tasks)
- **Circuit breakers**: `swarmpit.resilience` wraps external calls
- **Rate limiting**: `swarmpit.ratelimit` on login endpoint
- **SPA fallback**: Non-API routes serve index.html (no 404 on refresh)

## Frontend

- **Rum** (React wrapper for ClojureScript)
- **Material UI** v4 via cljsjs
- Single app atom with cursors (`swarmpit.component.state`)
- SSE for real-time events (`/events` endpoint)

## Docker

- Non-root container (user: `swarmpit`)
- `tini` as init process
- JVM flags: `-XX:+UseContainerSupport -XX:MaxRAMPercentage=75.0`
- Multi-arch: amd64, arm64, armv7

## Git Conventions

- Conventional Commits: `<type>(<scope>): <subject> (#issue)`
- Types: feat, fix, docs, style, refactor, test, chore
- Every commit references an issue
- Branch naming: `fix/<issue-number>-<description>`, `feat/<description>`

## CI/CD

- GitHub Actions (`.github/workflows/`)
- `build.yml`: test + build on push/PR to master
- `release.yml`: tag-triggered release + Docker push
- Multi-arch Docker builds via QEMU + Buildx

## Security Practices

- No secrets in code or environment defaults
- Rate limiting on authentication endpoints
- Generic error messages to API consumers (details logged server-side)
- Input sanitization on all shell-executed values
- Non-root container execution
- Circuit breakers prevent cascading failures
