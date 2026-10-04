from __future__ import annotations

import os
from typing import TYPE_CHECKING

import pytest

from app.core.config import Settings, clear_settings_cache
from app.core.logging import reset_logging

if TYPE_CHECKING:
    from collections.abc import Iterator


@pytest.fixture(autouse=True)
def _isolate_environment() -> Iterator[None]:
    snapshot = dict(os.environ)
    clear_settings_cache()
    reset_logging()
    yield
    clear_settings_cache()
    reset_logging()
    os.environ.clear()
    os.environ.update(snapshot)


@pytest.fixture
def settings() -> Settings:
    return Settings(_env_file=None)
