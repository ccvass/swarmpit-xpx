# How to Contribute

Swarmpit XPX is written in Clojure (backend) and ClojureScript (frontend with Rum/React). Data is stored in embedded SQLite. Docker is connected via socket.

## Prerequisites

- Java 21+ (Eclipse Temurin recommended)
- Leiningen 2.8.2+
- Docker socket accessible at `/var/run/docker.sock`

## Development Environment

```bash
lein deps          # Install dependencies
lein repl          # Start REPL
```

In the REPL, call `(fig-start)` to start the agent container and Figwheel dev server on http://localhost:3449.

For frontend REPL: `(cljs-repl)`. Both are in the `repl.user` namespace.

## Build

```bash
lein with-profile prod uberjar    # Build production JAR
docker build -t swarmpit-xpx .    # Build Docker image (multi-stage)
```

## Testing

```bash
lein test              # Unit tests
lein test :integration # Integration tests (requires Docker)
lein test :all         # All tests
```

## Reporting Issues

Create an issue at https://github.com/ccvass/swarmpit-xpx/issues with:

```
Steps to reproduce:
1. ...
2. ...

What happens:
- ...

What should happen:
- ...
```
