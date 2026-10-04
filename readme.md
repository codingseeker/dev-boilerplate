# Dev Boilerplates

My personal collection of reusable project foundations for Go, Python, and JavaScript.

These are meant to be used directly as starting points for new projects. The goal is to avoid repeating setup work and have a familiar, modular structure across the languages I use.

## Structure

```text
biolerplate/
├── go-starter/
├── python-starter/
├── javascript-starter/
└── README.md
```

Each directory is a standalone boilerplate with its own dependencies, configuration, commands, tests, and documentation.

## What They Have in Common

The boilerplates follow a similar overall approach while using idiomatic tools and conventions for each language.

- **CLI-first** — the command line is the primary interface
- **Modular structure** — application logic is separated from entry points and infrastructure
- **Configuration** — settings are managed centrally and can be provided through environment variables
- **Logging** — consistent and structured application logs
- **Error handling** — predictable handling of application and runtime errors
- **Testing** — unit and integration tests where appropriate
- **Optional REST API** — an HTTP interface can use the same application logic as the CLI
- **Maintainability** — clear separation of concerns and room to extend the application

## Choosing a Boilerplate

| Directory | Use when |
|---|---|
| `go-starter/` | Starting a Go application |
| `python-starter/` | Starting a Python application |
| `javascript-starter/` | Starting a JavaScript application |

Each boilerplate has its own README with setup instructions, available commands, configuration details, and testing steps.

## How I Use Them

1. Choose the language suited to the project.
2. Copy the corresponding starter into the new project directory.
3. Rename and modify the application to fit the project.
4. Remove features and dependencies that are not needed.
5. Run the tests and checks before building further.

These are starting points, not frameworks that every project must follow exactly. The structure should be adapted to the actual requirements.

## Notes

This is a personal reference and an evolving collection. I'll update the boilerplates as I learn better practices, encounter new requirements, or find improvements worth carrying over to future projects.
