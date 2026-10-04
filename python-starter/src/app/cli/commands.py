from __future__ import annotations

from typing import Annotated, Any

import typer
import uvicorn

from app import __version__
from app.api.app import create_app
from app.core.config import Environment, LogLevel, Settings, build_settings
from app.core.logging import configure_logging, get_logger

_logger = get_logger("cli")

app = typer.Typer(
    name="app",
    help="A production-grade, CLI-first, modular Python boilerplate.",
    add_completion=False,
    no_args_is_help=True,
    pretty_exceptions_show_locals=False,
)


def _version_callback(value: bool) -> None:
    if value:
        typer.echo(__version__)
        raise typer.Exit()


def _resolve_settings(
    env: Environment | None,
    log_level: LogLevel | None,
    **overrides: Any,
) -> Settings:
    return build_settings(environment=env, log_level=log_level, **overrides)


@app.callback()
def _callback(
    version: Annotated[
        bool,
        typer.Option(
            "--version",
            callback=_version_callback,
            is_eager=True,
            help="Show the application version and exit.",
        ),
    ] = False,
) -> None:
    pass


@app.command()
def run(
    name: Annotated[str, typer.Option("--name", "-n", help="Name to greet.")] = "world",
    env: Environment | None = typer.Option(None, "--env"),
    log_level: LogLevel | None = typer.Option(None, "--log-level"),
) -> None:
    settings = _resolve_settings(env, log_level)
    configure_logging(settings.log_level, json_mode=settings.is_production)

    from app.services.greeting import GreetingService
    from app.services.status import StatusService

    greeting = GreetingService(settings).greet(name)
    status = StatusService(settings).status()

    typer.echo(greeting.message)
    typer.echo(
        f"app={status.app_name} env={status.environment} "
        f"python={status.python_version} uptime={status.uptime_seconds}s"
    )


@app.command()
def serve(
    host: str | None = typer.Option(None, "--host", help="Override the HTTP bind host."),
    port: int | None = typer.Option(None, "--port", help="Override the HTTP bind port."),
    env: Environment | None = typer.Option(None, "--env"),
    log_level: LogLevel | None = typer.Option(None, "--log-level"),
) -> None:
    settings = _resolve_settings(env, log_level, http_host=host, http_port=port)
    configure_logging(settings.log_level, json_mode=settings.is_production)

    application = create_app(settings)
    _logger.info(
        "starting server",
        extra={"http_host": settings.http_host, "http_port": settings.http_port},
    )
    uvicorn.run(
        application,
        host=settings.http_host,
        port=settings.http_port,
        timeout_keep_alive=int(settings.http_read_timeout),
        log_level=settings.log_level.value.lower(),
    )
