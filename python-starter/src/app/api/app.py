from __future__ import annotations

from contextlib import asynccontextmanager
from typing import TYPE_CHECKING

from fastapi import FastAPI, Request
from fastapi.exceptions import RequestValidationError
from fastapi.responses import JSONResponse
from starlette.exceptions import HTTPException as StarletteHTTPException

from app import __version__
from app.api.middleware import RequestLoggingMiddleware, RequestSizeLimitMiddleware
from app.api.routes import router
from app.core.config import Settings, get_settings
from app.core.errors import AppError, ValidationError
from app.core.logging import configure_logging, get_logger
from app.infrastructure.tasks import BackgroundTaskManager
from app.services.analysis import TextAnalysisService
from app.services.greeting import GreetingService
from app.services.status import StatusService

if TYPE_CHECKING:
    from collections.abc import AsyncIterator

_logger = get_logger("api.app")


def create_app(settings: Settings | None = None) -> FastAPI:
    if settings is None:
        settings = get_settings()

    configure_logging(
        settings.log_level,
        json_mode=settings.is_production,
    )

    @asynccontextmanager
    async def lifespan(app: FastAPI) -> AsyncIterator[None]:
        _logger.info(
            "starting up",
            extra={"app_name": settings.app_name, "environment": settings.environment.value},
        )
        app.state.settings = settings
        app.state.greeting_service = GreetingService(settings)
        app.state.analysis_service = TextAnalysisService()
        app.state.status_service = StatusService(settings)
        app.state.task_manager = BackgroundTaskManager()
        try:
            yield
        finally:
            _logger.info("shutting down")
            await app.state.task_manager.shutdown()
            _logger.info("shutdown complete")

    app = FastAPI(
        title=settings.app_name,
        version=__version__,
        lifespan=lifespan,
        redirect_slashes=False,
    )

    app.add_middleware(RequestSizeLimitMiddleware, max_bytes=settings.max_request_bytes)
    app.add_middleware(RequestLoggingMiddleware)

    app.include_router(router)

    @app.exception_handler(AppError)
    async def _handle_app_error(request: Request, exc: AppError) -> JSONResponse:
        _logger.warning("application error", extra={"error": str(exc)})
        return JSONResponse(status_code=400, content={"error": str(exc)})

    @app.exception_handler(ValidationError)
    async def _handle_validation_error(request: Request, exc: ValidationError) -> JSONResponse:
        return JSONResponse(status_code=400, content={"error": str(exc)})

    @app.exception_handler(RequestValidationError)
    async def _handle_request_validation(
        request: Request, exc: RequestValidationError
    ) -> JSONResponse:
        return JSONResponse(
            status_code=422,
            content={"error": "Request validation failed", "details": exc.errors()},
        )

    @app.exception_handler(StarletteHTTPException)
    async def _handle_http_exception(request: Request, exc: StarletteHTTPException) -> JSONResponse:
        return JSONResponse(status_code=exc.status_code, content={"error": exc.detail})

    return app
