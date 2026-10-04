from __future__ import annotations

import asyncio

from app.infrastructure.tasks import BackgroundTaskManager


async def _sample_coro(duration: float = 0.01) -> str:
    await asyncio.sleep(duration)
    return "done"


class TestBackgroundTaskManager:
    async def test_spawn_tracks_task(self) -> None:
        manager = BackgroundTaskManager()
        manager.spawn(_sample_coro())
        assert manager.active_count == 1
        await manager.shutdown()
        assert manager.active_count == 0

    async def test_shutdown_cancels_pending(self) -> None:
        manager = BackgroundTaskManager()
        manager.spawn(_sample_coro(duration=10))
        assert manager.active_count == 1
        await manager.shutdown()
        assert manager.active_count == 0

    async def test_shutdown_without_tasks(self) -> None:
        manager = BackgroundTaskManager()
        await manager.shutdown()
        assert manager.active_count == 0

    async def test_multiple_tasks_all_cancelled(self) -> None:
        manager = BackgroundTaskManager()
        for _ in range(5):
            manager.spawn(_sample_coro(duration=10))
        assert manager.active_count == 5
        await manager.shutdown()
        assert manager.active_count == 0

    async def test_completed_task_discarded(self) -> None:
        manager = BackgroundTaskManager()
        task = manager.spawn(_sample_coro(duration=0.001))
        await asyncio.sleep(0.05)
        assert manager.active_count == 0
        assert task.result() == "done"
