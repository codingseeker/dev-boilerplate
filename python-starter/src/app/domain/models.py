from __future__ import annotations

from pydantic import BaseModel, Field


class Greeting(BaseModel):
    message: str
    name: str


class TextInput(BaseModel):
    text: str = Field(min_length=1, max_length=10_000)


class TextAnalysis(BaseModel):
    characters: int
    words: int
    lines: int


class StatusInfo(BaseModel):
    app_name: str
    environment: str
    python_version: str
    platform: str
    uptime_seconds: float
