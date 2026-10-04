from __future__ import annotations

from app.core.config import Settings
from app.domain.models import TextAnalysis
from app.services.analysis import TextAnalysisService
from app.services.greeting import GreetingService
from app.services.status import StatusService


class TestGreetingService:
    def test_greet_returns_message(self, settings: Settings) -> None:
        service = GreetingService(settings)
        greeting = service.greet("Alice")
        assert greeting.name == "Alice"
        assert greeting.message == "Hello, Alice!"

    def test_greet_uses_configured_word(self) -> None:
        settings = Settings(service_greeting="Greetings", _env_file=None)
        service = GreetingService(settings)
        assert service.greet("Bob").message == "Greetings, Bob!"

    def test_greet_strips_whitespace(self, settings: Settings) -> None:
        service = GreetingService(settings)
        assert service.greet("  Carol  ").name == "Carol"

    def test_greet_blank_name_defaults(self, settings: Settings) -> None:
        service = GreetingService(settings)
        assert service.greet("   ").name == "world"


class TestTextAnalysisService:
    def test_analyze_counts(self) -> None:
        service = TextAnalysisService()
        result = service.analyze("hello world\nfoo bar baz")
        assert result == TextAnalysis(characters=23, words=5, lines=2)

    def test_analyze_empty_lines(self) -> None:
        service = TextAnalysisService()
        result = service.analyze("a\n\nb")
        assert result.lines == 3
        assert result.words == 2

    def test_analyze_single_word(self) -> None:
        service = TextAnalysisService()
        result = service.analyze("word")
        assert result.characters == 4
        assert result.words == 1
        assert result.lines == 1


class TestStatusService:
    def test_status_fields(self, settings: Settings) -> None:
        service = StatusService(settings, started_at=100.0)

        info = service.status()
        assert info.app_name == "app"
        assert info.environment == "development"
        assert info.python_version
        assert info.platform
        assert info.uptime_seconds >= 0

    def test_uptime_increases(self, settings: Settings) -> None:
        service = StatusService(settings, started_at=0.0)
        first = service.status().uptime_seconds
        second = service.status().uptime_seconds
        assert second >= first
