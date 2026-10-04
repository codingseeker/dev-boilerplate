from __future__ import annotations

import pytest
from pydantic import ValidationError

from app.core.config import (
    Environment,
    LogLevel,
    Settings,
    build_settings,
    get_settings,
)


class TestDefaults:
    def test_default_values(self, settings: Settings) -> None:
        assert settings.app_name == "app"
        assert settings.environment is Environment.DEVELOPMENT
        assert settings.log_level is LogLevel.INFO
        assert settings.http_host == "127.0.0.1"
        assert settings.http_port == 8080
        assert settings.http_read_timeout == 30.0
        assert settings.http_write_timeout == 30.0
        assert settings.http_shutdown_timeout == 30.0
        assert settings.max_request_bytes == 1_048_576
        assert settings.service_greeting == "Hello"

    def test_is_production_false_by_default(self, settings: Settings) -> None:
        assert settings.is_production is False
        assert settings.is_testing is False


class TestEnvironmentVariables:
    def test_app_name_override(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.setenv("APP_NAME", "my-tool")
        assert Settings(_env_file=None).app_name == "my-tool"

    def test_app_env_override(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.setenv("APP_ENV", "production")
        settings = Settings(_env_file=None)
        assert settings.environment is Environment.PRODUCTION
        assert settings.is_production is True

    def test_log_level_override(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.setenv("LOG_LEVEL", "debug")
        assert Settings(_env_file=None).log_level is LogLevel.DEBUG

    def test_http_port_override(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.setenv("HTTP_PORT", "9000")
        assert Settings(_env_file=None).http_port == 9000

    def test_case_insensitive_env(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.setenv("APP_ENV", "PRODUCTION")
        assert Settings(_env_file=None).environment is Environment.PRODUCTION


class TestValidation:
    def test_invalid_port_rejected(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.setenv("HTTP_PORT", "70000")
        with pytest.raises(ValidationError):
            Settings(_env_file=None)

    def test_invalid_environment_rejected(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.setenv("APP_ENV", "staging")
        with pytest.raises(ValidationError):
            Settings(_env_file=None)

    def test_invalid_log_level_rejected(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.setenv("LOG_LEVEL", "verbose")
        with pytest.raises(ValidationError):
            Settings(_env_file=None)

    def test_non_positive_timeout_rejected(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.setenv("HTTP_READ_TIMEOUT", "0")
        with pytest.raises(ValidationError):
            Settings(_env_file=None)

    def test_empty_app_name_rejected(self) -> None:
        with pytest.raises(ValidationError):
            Settings(app_name="")


class TestPrecedence:
    def test_init_kwargs_beat_env(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.setenv("APP_NAME", "from-env")
        settings = Settings(app_name="from-kwarg", _env_file=None)
        assert settings.app_name == "from-kwarg"

    def test_build_settings_skips_none(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.setenv("APP_NAME", "from-env")
        settings = build_settings(environment=None)
        assert settings.app_name == "from-env"

    def test_build_settings_applies_overrides(self) -> None:
        settings = build_settings(app_name="override", http_port=9999)
        assert settings.app_name == "override"
        assert settings.http_port == 9999


class TestGetSettings:
    def test_get_settings_caches(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.setenv("APP_NAME", "cached")
        first = get_settings()
        monkeypatch.setenv("APP_NAME", "changed")
        second = get_settings()
        assert first is second
        assert second.app_name == "cached"

    def test_production_skips_dotenv(self, monkeypatch: pytest.MonkeyPatch, tmp_path) -> None:
        monkeypatch.setenv("APP_ENV", "production")
        monkeypatch.setenv("APP_NAME", "prod-app")
        settings = get_settings()
        assert settings.app_name == "prod-app"
        assert settings.is_production is True
