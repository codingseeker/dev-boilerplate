# Python Starter

A production-grade, CLI-first, modular Python boilerplate designed for
cybersecurity tooling, automation, system utilities, network tools, and
backend services. The architecture supports a REST API without duplicating
application or business logic.

## Features

- **CLI-first, API-ready** — one codebase, two transports (CLI + REST API)
- **Modular `src` layout** — clear separation of concerns
- **Centralized configuration** — `pydantic-settings` with environment overrides
- **Structured logging** — JSON in production, human-readable console in development
- **Graceful shutdown** — lifespan management with background task cleanup
- **Type-safe** — full type annotations with `mypy` strict mode
- **Well-tested** — comprehensive unit and integration tests
- **Secure by default** — request size limits, no secret leakage, safe error responses

## Architecture

```
.
├── src/app/
│   ├── __init__.py          # Package metadata
│   ├── __main__.py          # python -m app entry point
│   ├── main.py              # Console script entry point
│   ├── cli/                 # CLI commands (Typer)
│   ├── api/                 # REST API (FastAPI)
│   ├── core/                # Config, logging, errors
│   ├── domain/              # Domain models (Pydantic)
│   ├── services/            # Application services (shared business logic)
│   └── infrastructure/      # Technical capabilities (task management)
├── tests/
│   ├── unit/                # Unit tests
│   └── integration/         # Integration tests
└── .github/workflows/       # CI pipeline
```

The domain and application logic never depend on the CLI or HTTP framework.
Both transports are thin adapters over the same services.

## Prerequisites

- [uv](https://docs.astral.sh/uv/) package manager (recommended) or `pip`
- Python 3.11+
- Docker (optional)

## Installation

### Using uv (recommended)

```bash
git clone <repo-url>
cd python-starter
uv sync --extra dev
```

### Using pip

```bash
git clone <repo-url>
cd python-starter
python -m venv .venv
source .venv/bin/activate
pip install -e ".[dev]"
```

## Configuration

Configuration is resolved with the following precedence (highest first):

1. CLI flags
2. Environment variables
3. `.env` file (development only)
4. Defaults

The `.env` file is **not** loaded in production, so local configuration can
never leak into a production deployment.

| Variable | Default | Description |
|---|---|---|
| `APP_NAME` | `app` | Application name |
| `APP_ENV` | `development` | Environment (`development`, `testing`, `production`) |
| `LOG_LEVEL` | `info` | Log level (`debug`, `info`, `warning`, `error`) |
| `HTTP_HOST` | `127.0.0.1` | HTTP server bind address |
| `HTTP_PORT` | `8080` | HTTP server bind port |
| `HTTP_READ_TIMEOUT` | `30` | Request read timeout (seconds) |
| `HTTP_WRITE_TIMEOUT` | `30` | Response write timeout (seconds) |
| `HTTP_SHUTDOWN_TIMEOUT` | `30` | Graceful shutdown timeout (seconds) |
| `MAX_REQUEST_BYTES` | `1048576` | Maximum request body size (bytes) |
| `SERVICE_GREETING` | `Hello` | Greeting word for the demo service |

See `.env.example` for a complete example.

## CLI Usage

```bash
# Show help
app --help

# Show version
app --version

# Run the core application
app run
app run --name Alice

# Start the REST API server
app serve
app serve --host 0.0.0.0 --port 9000

# With environment overrides
app --env production run
app serve --env production --log-level debug
```

All commands are also available via `python -m app`:

```bash
python -m app run
python -m app serve
```

## API Endpoints

| Method | Path | Description |
|---|---|---|
| `GET` | `/health` | Liveness probe |
| `GET` | `/ready` | Readiness probe |
| `GET` | `/greet?name=...` | Greeting (demo service) |
| `POST` | `/analyze` | Text analysis (demo service) |
| `GET` | `/status` | Application status |

### Example

```bash
# Start the server
app serve

# Health check
curl http://localhost:8080/health

# Greeting
curl "http://localhost:8080/greet?name=Alice"

# Text analysis
curl -X POST http://localhost:8080/analyze \
  -H "Content-Type: application/json" \
  -d '{"text": "hello world"}'
```

## Development

### Setup

```bash
make dev
```

### Running checks

```bash
make format       # Format code with ruff
make lint         # Lint with ruff
make typecheck    # Type check with mypy
make test         # Run tests
make coverage     # Run tests with coverage
make build        # Build the package
```

### Running the application

```bash
make run          # Run CLI
make serve        # Start API server
```

## Testing

```bash
# Run all tests
uv run pytest

# Run with coverage
uv run pytest --cov --cov-report=term-missing

# Run specific test file
uv run pytest tests/unit/test_config.py
```

## Building and Packaging

```bash
make build
# or
uv build
```

This produces a wheel and sdist in `dist/`.

## Docker

```bash
# Build the image
docker build -t app .

# Run the container
docker run --rm -p 8080:8080 app

# Run with custom port
docker run --rm -p 9000:8080 -e HTTP_PORT=8080 app serve --port 8080
```

The image uses a non-root user and handles termination signals correctly.

## Security Considerations

- **No secrets in logs** — the logging framework never serializes environment
  variables or sensitive fields.
- **Request size limits** — oversized payloads are rejected with HTTP 413.
- **Safe error responses** — internal stack traces and secrets are never
  exposed to API clients.
- **No arbitrary code execution** — the template contains no `eval`,
  `exec`, `shell=True`, or unsafe deserialization.
- **Input validation** — all untrusted input is validated via Pydantic.
- **Dependency scanning** — CI runs `uv audit` to check for known
  vulnerabilities.

For a real deployment, you would additionally need:

- **Authentication/authorization** — add middleware or dependencies (e.g.,
  OAuth2, API keys) before exposing the API publicly.
- **CORS** — configure `CORSMiddleware` if the API is accessed from browsers.
- **TLS** — terminate TLS at a reverse proxy or load balancer.
- **Rate limiting** — add rate limiting for public endpoints.

## Extending the Template

### Adding a new service

1. Define domain models in `src/app/domain/models.py`.
2. Implement the service in `src/app/services/`, injecting `Settings`.
3. The service is now available to both the CLI and the API.

### Adding a new API endpoint

1. Add a route handler in `src/app/api/routes.py`.
2. Resolve the service via `Depends` from `request.app.state`.
3. Validate input with a Pydantic model.

### Adding a new CLI command

1. Add a command function in `src/app/cli/commands.py` with the `@app.command()`
   decorator.
2. Use `_resolve_settings` to layer CLI flags over environment variables.
3. Delegate to the shared services.

## License

A license decision is required. This project does not currently include a
`LICENSE` file.
