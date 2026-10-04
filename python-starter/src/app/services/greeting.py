from __future__ import annotations

from typing import TYPE_CHECKING

from app.domain.models import Greeting

if TYPE_CHECKING:
    from app.core.config import Settings


class GreetingService:
    def __init__(self, settings: Settings) -> None:
        self._settings = settings

    def greet(self, name: str) -> Greeting:
        cleaned = name.strip() or "world"
        return Greeting(message=f"{self._settings.service_greeting}, {cleaned}!", name=cleaned)
