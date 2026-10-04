from __future__ import annotations

import enum
import os
from functools import lru_cache
from typing import Any

from pydantic import AliasChoices, Field, field_validator
from pydantic_settings import BaseSettings, SettingsConfigDict


class Environment(enum.StrEnum):
    DEVELOPMENT = "development"
    TESTING = "testing"
    PRODUCTION = "production"


class LogLevel(enum.StrEnum):
    DEBUG = "DEBUG"
    INFO = "INFO"
    WARNING = "WARNING"
    ERROR = "ERROR"


class Settings(BaseSettings):
    model_config = SettingsConfigDict(
        env_file=".env",
        env_file_encoding="utf-8",
        extra="ignore",
        case_sensitive=False,
        populate_by_name=True,
    )

    app_name: str = Field(default="app", min_length=1, max_length=128)
    environment: Environment = Field(
        default=Environment.DEVELOPMENT,
        validation_alias=AliasChoices("APP_ENV", "ENVIRONMENT"),
    )
    log_level: LogLevel = Field(
        default=LogLevel.INFO,
        validation_alias=AliasChoices("LOG_LEVEL"),
    )

    http_host: str = Field(default="127.0.0.1", min_length=1)
    http_port: int = Field(default=8080, ge=1, le=65535)
    http_read_timeout: float = Field(default=30.0, gt=0, le=3600)
    http_write_timeout: float = Field(default=30.0, gt=0, le=3600)
    http_shutdown_timeout: float = Field(default=30.0, gt=0, le=3600)

    max_request_bytes: int = Field(default=1_048_576, ge=1)

    service_greeting: str = Field(default="Hello", min_length=1, max_length=64)

    @field_validator("environment", mode="before")
    @classmethod
    def _coerce_environment(cls, value: object) -> object:
        if isinstance(value, str):
            return value.strip().lower()
        return value

    @field_validator("log_level", mode="before")
    @classmethod
    def _coerce_log_level(cls, value: object) -> object:
        if isinstance(value, str):
            return value.strip().upper()
        return value

    @property
    def is_production(self) -> bool:
        return self.environment is Environment.PRODUCTION

    @property
    def is_testing(self) -> bool:
        return self.environment is Environment.TESTING


@lru_cache(maxsize=1)
def get_settings() -> Settings:
    if os.environ.get("APP_ENV", "").strip().lower() == "production":
        return Settings(_env_file=None)  # type: ignore[call-arg]
    return Settings()


def clear_settings_cache() -> None:
    get_settings.cache_clear()


def build_settings(**overrides: Any) -> Settings:
    kwargs: dict[str, Any] = {k: v for k, v in overrides.items() if v is not None}
    return Settings(**kwargs)
