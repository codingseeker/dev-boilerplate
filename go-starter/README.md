# go-starter

A production-grade, CLI-first Go boilerplate. One application, one set of
services, two entry points: a command line interface and a REST API. Business
logic is written once and reused by both.

The template has **no third-party dependencies**: everything is built on the Go
standard library (`flag`, `log/slog`, `net/http`, `os/signal`, `testing`).

## Contents

- [Features](#features)
- [Requirements](#requirements)
- [Installation](#installation)
- [Quick start](#quick-start)
- [Architecture](#architecture)
- [Configuration](#configuration)
- [CLI usage](#cli-usage)
- [API usage](#api-usage)
- [Graceful shutdown](#graceful-shutdown)
- [Logging](#logging)
- [Development](#development)
- [Testing](#testing)
- [Build](#build)
- [Docker](#docker)
- [Security considerations](#security-considerations)
- [Extending the template](#extending-the-template)

## Features

- Modular, dependency-injected application layer shared by the CLI and the API
- Validated configuration with a documented precedence order and descriptive errors
- Structured logging with `log/slog`: readable text in development, JSON in production
- Signal handling for `SIGINT`/`SIGTERM` with bounded, graceful shutdown
- Minimal HTTP API with health, readiness and demonstration endpoints
- Middleware for panic recovery, request ids, access logs, body limits and safe headers
- Table-driven unit tests, `httptest` based handler tests and black box integration tests
- Makefile, GitHub Actions pipeline, `golangci-lint` configuration and a multi-stage Dockerfile

## Requirements

- Go 1.25 or newer
- Docker (optional)
- `golangci-lint` v2 (optional, `make lint` falls back to `go vet` and `gofmt`)

## Installation

```bash
git clone <repository-url> go-starter
cd go-starter
go mod download
```

## Quick start

```bash
# Core application
go run ./cmd/app run

# HTTP API
go run ./cmd/app serve

# In another shell
curl -s localhost:8080/health
curl -s 'localhost:8080/v1/greet?name=ada'

# Stop with Ctrl+C; in-flight requests finish first
```

## Architecture

```text
go-starter/
├── cmd/app/                 # Single entry point: wires the CLI, maps errors to exit codes
├── internal/
│   ├── app/                 # Application lifecycle: DI, signals, startup, graceful shutdown
│   ├── buildinfo/           # Version metadata injected through -ldflags
│   ├── cli/                 # Argument parsing and exit codes, no business logic
│   ├── config/              # Configuration resolution and validation
│   ├── domain/              # Core types and contracts, free of CLI and HTTP concerns
│   ├── logger/              # The single structured logger, carried through context
│   ├── platform/dotenv/     # Minimal .env reader for local development
│   ├── server/              # HTTP routing, middleware, validation, server lifecycle
│   └── service/             # Application capabilities implementing the domain contracts
├── tests/                   # Black box integration tests (binary and end-to-end API)
├── .github/workflows/ci.yml # Formatting, vet, lint, tests, race, build, vulnerability scan
├── .env.example
├── .golangci.yml
├── Dockerfile
├── Makefile
└── go.mod
```

Dependency direction (arrows point from caller to callee):

```text
cmd/app → cli → app → service → domain
                     ↓        ↑
                   server ────┘
                     ↓
                  config, logger, platform/dotenv
```

Rules that keep the template healthy:

- `domain` and `service` never import `cli` or `server`. Transports depend on the
  `domain.Service` interface, never on a concrete implementation.
- `cli` only parses arguments and calls the application layer; it contains no business logic.
- `server` only owns HTTP concerns: routing, middleware, request validation, response shaping.
- No package keeps mutable state in globals. The logger, the environment lookup and the
  signal strategy are injected, which is what makes the tests deterministic.

## Configuration

Every setting can come from a CLI flag, an environment variable or a default.

| Variable | Flag | Default | Description |
|---|---|---|---|
| `APP_NAME` | `--app-name` | `go-starter` | Application name |
| `APP_ENV` | `--env` | `development` | `development`, `testing` or `production` |
| `LOG_LEVEL` | `--log-level` | `info` | `debug`, `info`, `warn` or `error` |
| `HTTP_HOST` | `--http-host` | `0.0.0.0` | Listen host |
| `HTTP_PORT` | `--http-port` | `8080` | Listen port (1-65535) |
| `HTTP_READ_TIMEOUT` | `--http-read-timeout` | `30s` | Read timeout, also used for headers |
| `HTTP_WRITE_TIMEOUT` | `--http-write-timeout` | `30s` | Write timeout |
| `HTTP_IDLE_TIMEOUT` | `--http-idle-timeout` | `120s` | Keep-alive idle timeout |
| `HTTP_SHUTDOWN_TIMEOUT` | `--http-shutdown-timeout` | `30s` | Graceful shutdown budget |

### Precedence

```text
CLI flags  >  environment variables  >  .env file  >  built-in defaults
```

- Blank values (`""` or whitespace only) are treated as "not set" and fall through
  to the next source.
- Durations accept Go syntax (`30s`, `1m30s`) and bare integers as seconds (`30`).
- Invalid values fail fast with a message that names the variable and the value,
  for example `invalid HTTP_PORT "abc": must be an integer between 1 and 65535`.

### The `.env` file

`.env` is optional and only exists to ease local development:

- It is looked up as `.env` in the working directory, or at `--env-file <path>`.
- Variables already present in the environment always win over the file.
- The file is **never** loaded when the resolved environment is `production`.
  Passing `--env-file` together with `APP_ENV=production` is an error, so a
  production deployment cannot silently pick up developer settings.
- `.env.example` documents every variable and contains no secrets.

Copy `.env.example` to `.env` to start from the defaults:

```bash
cp .env.example .env
```

## CLI usage

```text
Usage:
  go-starter <command> [flags]

Commands:
  run       Run the core application
  serve     Start the HTTP REST API server
  version   Print the version and exit
  help      Print this help and exit

Global flags:
  -h, --help      Print this help and exit
  -v, --version   Print the version and exit
```

Examples:

```bash
go run ./cmd/app --help
go run ./cmd/app version
go run ./cmd/app run --env=testing --log-level=debug
go run ./cmd/app serve --http-host=127.0.0.1 --http-port=8081
go run ./cmd/app serve --env-file=./config/dev.env
go run ./cmd/app serve -h          # flags of a single command
```

`run` performs the core work once and then stays alive until it is signalled, so the
process behaves like a long-running service and shutdown code is exercised in real
usage. `serve` starts the HTTP API described below.

Exit codes:

| Code | Meaning |
|---|---|
| `0` | Success, including `--help`, `version` and a shutdown triggered by a signal |
| `1` | Runtime failure, for example the port is already in use |
| `2` | Usage or configuration failure: unknown command, unknown flag, invalid value |

## API usage

| Method | Path | Description |
|---|---|---|
| `GET` | `/health` | Liveness. `200` while the process can serve traffic |
| `GET` | `/ready` | Readiness. `503` while the instance is draining |
| `GET` | `/v1/greet?name=ada` | Demonstration operation, `400` for a missing or invalid name |
| `POST` | `/v1/echo` | Echoes `{"message":"..."}`, `400` for invalid JSON, `413` for oversized bodies, `415` for a wrong content type |

```bash
curl -s localhost:8080/health
# {"app":"go-starter","checks":{"config":"ok"},"environment":"development","status":"ok","version":"dev"}

curl -s localhost:8080/v1/greet?name=ada
# {"message":"hello ada, go-starter says welcome to development","name":"ada"}

curl -s -XPOST localhost:8080/v1/echo -H 'Content-Type: application/json' -d '{"message":"ping"}'
# {"message":"ping","name":"echo"}

curl -s localhost:8080/v1/greet
# {"error":{"code":"invalid_request","message":"greet: invalid name: name must not be empty","request_id":"..."}}
```

Every error uses the same shape and never contains internal details:

```json
{
  "error": {
    "code": "invalid_request",
    "message": "human readable, safe to show",
    "request_id": "6f1c..."
  }
}
```

Error codes: `invalid_request`, `invalid_json`, `not_found`, `method_not_allowed`,
`body_too_large`, `unsupported_media_type`, `service_unavailable`, `internal_error`.

Operational details:

- Request bodies are capped at 1 MiB by `http.MaxBytesReader`.
- Headers are capped at 1 MiB, and every request gets read, write, header-read
  and idle timeouts.
- `X-Request-Id` is echoed when the caller supplies one and generated otherwise;
  the same value appears in the logs and in error payloads.
- Responses carry `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`,
  `Referrer-Policy: no-referrer` and `Cache-Control: no-store`.
- A panic in a handler becomes a `500` with a generic body; the stack trace stays
  in the logs.

## Graceful shutdown

1. `SIGINT` or `SIGTERM` cancels the application context through
   `signal.NotifyContext`.
2. Readiness flips to `503` so load balancers stop sending new traffic.
3. The HTTP server drains in-flight requests within `HTTP_SHUTDOWN_TIMEOUT`.
4. Remaining resources are released in reverse construction order.
5. Remaining connections are force closed if the deadline expires.

Startup failures are safe: a failed bind is reported as an error with a non-zero
exit code after resources have been released. Shutdown failures are collected with
`errors.Join`, so one broken resource cannot hide another. Shutdown behaviour is
fully testable because the signal strategy is injectable; the tests cancel a
context instead of raising real signals.

## Logging

- One logger per process, created once in the CLI.
- `development` and `testing`: text output with the source location, for humans.
- `production`: JSON output without source locations, for log aggregation.
- Levels: `debug`, `info`, `warn`, `error`.
- Every record carries `app` and `environment`; requests add `request_id`,
  `method`, `path`, `status`, `bytes`, `remote_addr` and `duration`.
- The logger travels through `context.Context` (`logger.WithContext` /
  `logger.FromContext`), so request scoped fields are added once.
- Request bodies and query values are never logged, and the configuration
  contains no secrets.

## Development

```bash
make fmt          # gofmt -s -w .
make fmt-check    # fail when formatting is missing
make vet          # go vet ./...
make lint         # golangci-lint run ./... (falls back to vet + fmt-check)
make test         # go test -count=1 ./...
make test-race    # go test -race -count=1 ./...
make test-short   # skip the slow binary integration tests
make cover        # coverage summary
make tidy         # go mod tidy
make verify       # fmt-check + vet + lint + race tests + build
make help         # list every target
```

## Testing

```bash
go test ./...                 # all tests
go test -race ./...           # with the race detector
go test -short ./...          # without the binary integration tests
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out
```

What the suite covers:

| Area | Examples |
|---|---|
| Configuration | defaults, overrides, invalid values, duration parsing, every variable |
| Logging | levels, text versus JSON, context propagation, invalid levels |
| Domain and services | name validation, greeting, readiness while draining, concurrency |
| Application | cancellation, startup failure, shutdown failure, joined errors, goroutine leaks |
| CLI | help, version, unknown commands, flags, precedence, exit codes, `.env` handling |
| HTTP | health, readiness, validation, body limit, method handling, error shape, header limits |
| Integration | end-to-end API plus the built binary, exit codes, `SIGINT` and `SIGTERM` |

All tests are deterministic, use no external services and no network access.

## Build

```bash
make build
# bin/go-starter, statically linked, with version metadata

make version    # build and print the injected version

go build ./...  # compile every package
```

Version metadata is injected at link time:

```bash
go build -ldflags "\
  -X github.com/codingseeker/go-starter/internal/buildinfo.Version=1.2.3 \
  -X github.com/codingseeker/go-starter/internal/buildinfo.Commit=$(git rev-parse --short HEAD) \
  -X github.com/codingseeker/go-starter/internal/buildinfo.BuildDate=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  -o bin/go-starter ./cmd/app
```

## Docker

```bash
make docker-build
docker run --rm -p 8080:8080 go-starter:dev serve

# or directly
docker build -t go-starter .
docker run --rm -p 8080:8080 \
  -e APP_ENV=production -e LOG_LEVEL=info -e HTTP_PORT=8080 \
  go-starter
```

The image is multi-stage: a builder stage produces a static `CGO_ENABLED=0`
binary, and the runtime stage is Alpine with `ca-certificates` and `tzdata`. The
process runs as the non-root user `app` (uid 10001) with a non-login shell, and
the default environment is production-oriented. Override the entrypoint
argument to run other commands, for example `docker run --rm go-starter run`.

## Security considerations

Included in the template:

- No hardcoded credentials, no secrets in configuration, logs or error payloads.
- Strict configuration validation; the process refuses to start on bad values.
- Bounded request bodies, headers, read/write/idle timeouts and a shutdown budget.
- Panic recovery, so a handler bug cannot take the process down or leak a stack trace.
- `http.Server` used with explicit timeouts, which avoids the slowloris class of
  attacks that the zero-value server is vulnerable to.
- Non-root container user, no shell tooling in the runtime image.
- `.env` loading disabled in production.

Deliberately out of scope for a generic template, because they need a real
deployment to be meaningful. Add them before exposing the service:

- **Authentication and authorisation.** Every endpoint is currently public. Put
  the API behind an identity-aware proxy or add token validation middleware, and
  authorise per route.
- **Authorisation of the demonstration endpoints.** `POST /v1/echo` exists to
  demonstrate body handling; remove or restrict it in a real product.
- **TLS.** Terminate TLS at the ingress, or wrap the listener with
  `tls.NewListener` and set `HTTP_HOST`/`HTTP_PORT` accordingly.
- **Rate limiting and quotas** per client or route.
- **CORS.** No CORS headers are sent. Browsers block cross-origin calls by
  default; add an explicit allowlist only if a browser client needs it.
- **Trusted proxies.** `X-Forwarded-For` and `X-Real-Ip` are logged as provided by
  the caller. Configure the proxy to overwrite them, or parse them only for known
  proxy addresses.
- **Dependency and image scanning** in the release pipeline
  (`govulncheck`, `trivy`), plus image signing.
- **Audit logging** for security relevant actions.

## Extending the template

1. **Add business logic.** Put types and rules in `internal/domain`, implement
   them in `internal/service`, and extend the `domain.Service` interface. Both
   transports pick the new capability up automatically.
2. **Add an endpoint.** Register the route in `internal/server/handlers.go`,
   validate the request, call the service with `r.Context()` and answer through
   `writeJSON` or `writeError`. Never expose an internal error string.
3. **Add a command.** Add a case in `internal/cli/cli.go` that prepares the shared
   dependencies and calls the application layer; return a `UsageError` for input
   problems so the exit code stays meaningful.
4. **Add configuration.** Add the field, the constant, an entry in `config.Keys`,
   the parsing in `LoadFromEnv`, a flag in `internal/cli` and a test case in
   `internal/config/config_test.go`.
5. **Add middleware.** Register it in `routes()` in the documented order; the
   first entry is the outermost wrapper.
6. **Add a dependency.** Keep it out of `domain` and `service`; inject it into
   `service.New` so tests can substitute a fake.

## License

No license file is included. Add one before publishing: the terms are a decision
for the project owner, not a default of this template.
