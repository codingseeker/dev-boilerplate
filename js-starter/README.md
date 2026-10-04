# JS Starter Boilerplate

Production-grade JavaScript CLI-first modular boilerplate for building cybersecurity tooling, automation, system utilities, network tools and backend services.

## Features

- CLI-first with modular architecture
- REST API support with Fastify
- Shared business logic between CLI and API
- Centralized configuration with dotenv and Zod
- Structured logging with Pino
- Graceful shutdown and lifecycle management
- Node.js built-in test runner
- ESLint + Prettier
- Coverage reporting with c8

## Prerequisites

- Node.js >= 20.0.0
- npm (or pnpm/yarn if adapted)

## Installation

```bash
npm ci
```

## Configuration

Configuration uses environment variables with validation. Copy `.env.example` and customize:

```bash
cp .env.example .env
```

Configuration precedence: CLI flags override environment variables override defaults.

## CLI Usage

```bash
# Show help
npm run cli -- --help
# or
node src/cli/index.js --help
# or (after linking)
app --help

# Run core application
npm run cli -- run
node src/cli/index.js run
app run

# Start server
npm run cli -- serve
npm run cli -- --serve
npm run serve
node src/cli/index.js --serve
app --serve
```

## API Usage

```bash
# Start server
npm run serve

# Health check
curl http://localhost:3000/health

# Readiness check
curl http://localhost:3000/ready

# Demo endpoint
curl http://localhost:3000/demo
```

## Development

```bash
npm run dev  # with watch mode
npm test     # run tests
npm run lint
npm run format
```

## Architecture

The codebase follows a modular architecture with clear separation:

- `src/core/` - Configuration, logging, errors
- `src/cli/` - CLI commands and parsing
- `src/api/` - HTTP server, routes, middleware
- `src/services/` - Business logic shared by CLI and API
- `src/domain/`, `src/infrastructure/`, `src/utils/` - Extension points

CLI and API share the same application services. Business logic does not depend on CLI or HTTP framework.
