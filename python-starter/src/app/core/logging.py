from __future__ import annotations

import contextvars
import json
import logging
import sys
from datetime import UTC, datetime
from typing import Any

from app.core.config import LogLevel

_LOGGER_NAME = "app"

_RESERVED_ATTRS = frozenset(
    {
        "name",
        "msg",
        "args",
        "levelname",
        "levelno",
        "pathname",
        "filename",
        "module",
        "exc_info",
        "exc_text",
        "stack_info",
        "lineno",
        "funcName",
        "created",
        "msecs",
        "relativeCreated",
        "thread",
        "threadName",
        "processName",
        "process",
        "message",
        "asctime",
        "taskName",
    }
)

request_id: contextvars.ContextVar[str | None] = contextvars.ContextVar("request_id", default=None)

_configured = False


class StructuredFormatter(logging.Formatter):
    def _extra_fields(self, record: logging.LogRecord) -> dict[str, Any]:
        return {
            key: value
            for key, value in record.__dict__.items()
            if key not in _RESERVED_ATTRS and not key.startswith("_")
        }

    def _common(self, record: logging.LogRecord) -> dict[str, Any]:
        data: dict[str, Any] = {
            "level": record.levelname,
            "logger": record.name,
            "message": record.getMessage(),
        }
        rid = request_id.get()
        if rid is not None:
            data["request_id"] = rid
        data.update(self._extra_fields(record))
        return data


class ConsoleFormatter(StructuredFormatter):
    def format(self, record: logging.LogRecord) -> str:
        data = self._common(record)
        timestamp = datetime.now(UTC).strftime("%Y-%m-%dT%H:%M:%S%z")
        line = f"{timestamp} [{data['level']}] {data['logger']}: {data['message']}"
        extras = {
            key: value for key, value in data.items() if key not in {"level", "logger", "message"}
        }
        if extras:
            line += " " + " ".join(f"{key}={value}" for key, value in extras.items())
        if record.exc_info:
            line += "\n" + self.formatException(record.exc_info)
        return line


class JsonFormatter(StructuredFormatter):
    def format(self, record: logging.LogRecord) -> str:
        data = self._common(record)
        data["timestamp"] = datetime.now(UTC).isoformat()
        if record.exc_info:
            data["exception"] = self.formatException(record.exc_info)
        return json.dumps(data, default=str)


def configure_logging(
    level: LogLevel | str = LogLevel.INFO,
    *,
    json_mode: bool = False,
) -> logging.Logger:
    global _configured

    level_name = level.value if isinstance(level, LogLevel) else str(level).upper()

    logger = logging.getLogger(_LOGGER_NAME)
    if _configured:
        logger.setLevel(level_name)
        return logger

    handler = logging.StreamHandler(sys.stderr)
    handler.setFormatter(JsonFormatter() if json_mode else ConsoleFormatter())
    logger.addHandler(handler)
    logger.setLevel(level_name)
    logger.propagate = False
    _configured = True
    return logger


def get_logger(name: str | None = None) -> logging.Logger:
    if name:
        return logging.getLogger(f"{_LOGGER_NAME}.{name}")
    return logging.getLogger(_LOGGER_NAME)


def reset_logging() -> None:
    global _configured
    logger = logging.getLogger(_LOGGER_NAME)
    for handler in logger.handlers[:]:
        logger.removeHandler(handler)
    logger.setLevel(logging.NOTSET)
    _configured = False
