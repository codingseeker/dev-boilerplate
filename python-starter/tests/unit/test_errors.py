from __future__ import annotations

import pytest

from app.core.errors import (
    AppError,
    ConfigurationError,
    NotFoundError,
    ServiceError,
    ValidationError,
)


class TestExceptionHierarchy:
    def test_all_inherit_app_error(self) -> None:
        assert issubclass(ConfigurationError, AppError)
        assert issubclass(ServiceError, AppError)
        assert issubclass(NotFoundError, AppError)
        assert issubclass(ValidationError, AppError)

    def test_not_found_is_service_error(self) -> None:
        assert issubclass(NotFoundError, ServiceError)

    def test_catch_as_app_error(self) -> None:
        with pytest.raises(AppError):
            raise ConfigurationError("bad config")

    def test_message_preserved(self) -> None:
        exc = ValidationError("invalid input")
        assert str(exc) == "invalid input"
