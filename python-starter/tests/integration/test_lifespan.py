from __future__ import annotations

import asyncio

from fastapi.testclient import TestClient

from app.api.app import create_app
from app.core.config import Settings
from app.infrastructure.tasks import BackgroundTaskManager


class TestLifespan:
    def test_startup_and_shutdown_run(self) -> None:
        settings = Settings(_env_file=None, environment="testing")
        app = create_app(settings)
        with TestClient(app) as client:
            response = client.get("/health")
            assert response.status_code == 200

    def test_services_available_during_request(self) -> None:
        settings = Settings(_env_file=None, environment="testing")
        app = create_app(settings)
        with TestClient(app):
            assert hasattr(app.state, "greeting_service")
            assert hasattr(app.state, "analysis_service")
            assert hasattr(app.state, "status_service")
            assert hasattr(app.state, "task_manager")

    async def test_shutdown_cancels_background_tasks(self) -> None:
        manager = BackgroundTaskManager()

        async def _long_running() -> None:
            await asyncio.sleep(60)

        manager.spawn(_long_running())
        assert manager.active_count == 1
        await manager.shutdown()
        assert manager.active_count == 0


class TestAppFactory:
    def test_create_app_with_explicit_settings(self) -> None:
        settings = Settings(_env_file=None, app_name="custom-app")
        app = create_app(settings)
        assert app.title == "custom-app"

    def test_create_app_without_settings_uses_env(self) -> None:
        app = create_app()
        assert app is not None
