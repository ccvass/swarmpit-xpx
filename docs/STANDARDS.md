# Development Standards

Project-specific development rules for Swarmpit XPX.

## Language and Runtime

- **Go** 1.26+ (backend)
- **ClojureScript** 1.11.132 (legacy frontend, compiled to static JS)
- **JavaScript** ES5 (xpx-features.js — XPX feature overlay)
- **SQLite** (embedded database via mattn/go-sqlite3)

## Code Style

- Follow standard Go idioms and `go vet` rules
- Package structure: `cmd/swarmpit/` (entry point), `internal/api/` (handlers, mappers, SSE), `internal/auth/` (JWT), `internal/store/` (SQLite), `internal/docker/` (Docker SDK wrapper)
- Exported functions for HTTP handlers (`func ServiceList(w, r)`)
- Unexported helpers with descriptive names (`func resolveServiceID`, `func sanitizeImageRef`)
- Error wrapping with `fmt.Errorf("context: %w", err)`

## Formatting and Linting

- `go vet ./...` before every commit
- `go test ./... -race` in CI
- `.golangci.yml` present for local linting (CI uses `go vet` due to Go 1.26 compatibility)

## Testing

- Unit tests in `*_test.go` files alongside source
- `internal/store/sqlite_test.go` — 20 tests, 91.8% coverage
- `internal/auth/jwt_test.go` — 8 tests, 100% coverage
- `internal/api/api_test.go` — 12 tests, pure logic (no Docker dependency)
- Run: `go test ./... -race -cover`

## Architecture

- **Single binary**: Go binary serves API + static frontend
- **Embedded SQLite**: No external database; data at `$SWARMPIT_DB_PATH/swarmpit.db`
- **In-memory stats**: Ring buffers for timeseries; agent pushes via `POST /events`
- **Docker socket**: Docker SDK client with 15s timeout and API version negotiation
- **SSE**: Subscription-based server-sent events with 30s refresh; authenticated
- **SPA fallback**: Non-API routes serve `index.html`

## Frontend

- Legacy ClojureScript SPA (compiled, served as static files)
- `xpx-features.js` — vanilla JS overlay adding: floating Tools panel, GitOps UI, image updates, system prune, compose auto-fill, ANSI log colors, piechart fix, update order toggle

## Security

- JWT authentication on all API and SSE endpoints
- Role-based access: `admin`, `user`, `viewer`
- `WriteOnly` middleware blocks viewers from write operations
- Registry credentials redacted in API responses (`••••••`)
- GitOps: path traversal prevention, webhook signature validation
- Image digest sanitization before Docker API calls
- Backup/restore preserves password hashes
- OAuth2/OIDC support for external authentication
- TOTP/2FA optional per user

## Docker

- `Dockerfile.xpx` — 3-stage build (ClojureScript frontend → Go binary → Alpine runtime)
- Version injected via `--build-arg VERSION` and `-ldflags "-X main.version=..."`
- `HEALTHCHECK` calls `/health/live` every 30s
- Multi-arch: amd64, arm64
- Trivy security scanning in CI

## Deployment

- Docker Swarm service with `--with-registry-auth`
- Swarmex compatible: requires `team` label + `--limit-memory`
- Stack deploys use `--with-registry-auth` automatically

## Git Conventions

- Conventional Commits: `<type>(<scope>): <subject> (#issue)`
- Types: feat, fix, docs, style, refactor, test, chore
- Every commit references an issue
- Branch naming: `fix/<issue-number>-<description>`, `feat/<description>`

## CI/CD

- GitHub Actions (`.github/workflows/`)
- `build.yml`: compile check on push/PR
- `release.yml`: tag-triggered — Go vet, test, multi-arch Docker build, Trivy scan, GitHub release
- Legacy Clojure job runs with `continue-on-error: true`
