from __future__ import annotations

from app.domain.models import TextAnalysis


class TextAnalysisService:
    def analyze(self, text: str) -> TextAnalysis:
        return TextAnalysis(
            characters=len(text),
            words=len(text.split()),
            lines=len(text.splitlines()),
        )
