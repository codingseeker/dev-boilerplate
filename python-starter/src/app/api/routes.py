from __future__ import annotations

from typing import TYPE_CHECKING, cast

from fastapi import APIRouter, Depends, Query, Request

from app.domain.models import Greeting, StatusInfo, TextAnalysis, TextInput

if TYPE_CHECKING:
    from app.services.analysis import TextAnalysisService
    from app.services.greeting import GreetingService
    from app.services.status import StatusService

router = APIRouter()


def get_greeting_service(request: Request) -> GreetingService:
    return cast("GreetingService", request.app.state.greeting_service)


def get_analysis_service(request: Request) -> TextAnalysisService:
    return cast("TextAnalysisService", request.app.state.analysis_service)


def get_status_service(request: Request) -> StatusService:
    return cast("StatusService", request.app.state.status_service)


@router.get("/health", response_model=dict[str, str])
async def health() -> dict[str, str]:
    return {"status": "ok"}


@router.get("/ready", response_model=dict[str, object])
async def ready(
    status_service: StatusService = Depends(get_status_service),
) -> dict[str, object]:
    info = status_service.status()
    return {"status": "ready", "app_name": info.app_name, "environment": info.environment}


@router.get("/greet", response_model=Greeting)
async def greet(
    name: str = Query(default="world", min_length=1, max_length=64),
    service: GreetingService = Depends(get_greeting_service),
) -> Greeting:
    return service.greet(name)


@router.post("/analyze", response_model=TextAnalysis)
async def analyze(
    payload: TextInput,
    service: TextAnalysisService = Depends(get_analysis_service),
) -> TextAnalysis:
    return service.analyze(payload.text)


@router.get("/status", response_model=StatusInfo)
async def status(
    status_service: StatusService = Depends(get_status_service),
) -> StatusInfo:
    return status_service.status()
