from __future__ import annotations

import json
import logging

from app.core.config import LogLevel
from app.core.logging import (
    ConsoleFormatter,
    JsonFormatter,
    configure_logging,
    get_logger,
    request_id,
    reset_logging,
)


class TestConfigureLogging:
    def test_returns_application_logger(self) -> None:
        logger = configure_logging(LogLevel.DEBUG)
        assert logger.name == "app"
        assert logger.level == logging.DEBUG

    def test_idempotent_no_duplicate_handlers(self) -> None:
        configure_logging(LogLevel.INFO)
        logger = logging.getLogger("app")
        handler_count = len(logger.handlers)
        configure_logging(LogLevel.WARNING)
        assert len(logger.handlers) == handler_count

    def test_level_adjusts_on_reconfigure(self) -> None:
        logger = configure_logging(LogLevel.INFO)
        assert logger.level == logging.INFO
        configure_logging(LogLevel.ERROR)
        assert logger.level == logging.ERROR

    def test_reset_logging_removes_handlers(self) -> None:
        configure_logging(LogLevel.INFO)
        reset_logging()
        logger = logging.getLogger("app")
        assert logger.handlers == []


class TestJsonFormatter:
    def test_emits_valid_json(self) -> None:
        formatter = JsonFormatter()
        record = logging.LogRecord("app.test", logging.INFO, __file__, 1, "hello", (), None)
        payload = json.loads(formatter.format(record))
        assert payload["message"] == "hello"
        assert payload["level"] == "INFO"
        assert payload["logger"] == "app.test"
        assert "timestamp" in payload

    def test_includes_request_id(self) -> None:
        token = request_id.set("abc123")
        try:
            formatter = JsonFormatter()
            record = logging.LogRecord("app.test", logging.INFO, __file__, 1, "hello", (), None)
            payload = json.loads(formatter.format(record))
            assert payload["request_id"] == "abc123"
        finally:
            request_id.reset(token)

    def test_includes_exception(self) -> None:
        formatter = JsonFormatter()
        try:
            raise ValueError("boom")
        except ValueError:
            import sys

            record = logging.LogRecord(
                "app.test", logging.ERROR, __file__, 1, "failed", (), sys.exc_info()
            )
        payload = json.loads(formatter.format(record))
        assert "ValueError" in payload["exception"]


class TestConsoleFormatter:
    def test_human_readable_output(self) -> None:
        formatter = ConsoleFormatter()
        record = logging.LogRecord("app.test", logging.INFO, __file__, 1, "hello world", (), None)
        output = formatter.format(record)
        assert "INFO" in output
        assert "hello world" in output
        assert "app.test" in output


class TestGetLogger:
    def test_nested_logger_name(self) -> None:
        logger = get_logger("api.routes")
        assert logger.name == "app.api.routes"

    def test_default_logger(self) -> None:
        assert get_logger().name == "app"
