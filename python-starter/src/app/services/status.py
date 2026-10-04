from __future__ import annotations

import platform
import sys
import time
from typing import TYPE_CHECKING

from app.domain.models import StatusInfo

if TYPE_CHECKING:
    from app.core.config import Settings


class StatusService:
    def __init__(self, settings: Settings, started_at: float | None = None) -> None:
        self._settings = settings
        self._started_at = started_at if started_at is not None else time.monotonic()

    def status(self) -> StatusInfo:
        return StatusInfo(
            app_name=self._settings.app_name,
            environment=self._settings.environment.value,
            python_version=sys.version.split()[0],
            platform=platform.system(),
            uptime_seconds=round(time.monotonic() - self._started_at, 3),
        )
