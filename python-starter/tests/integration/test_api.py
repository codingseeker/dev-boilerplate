from __future__ import annotations

import pytest
from fastapi.testclient import TestClient

from app.api.app import create_app
from app.core.config import Settings


@pytest.fixture
def client() -> TestClient:
    settings = Settings(_env_file=None, environment="testing")
    app = create_app(settings)
    with TestClient(app) as test_client:
        yield test_client


class TestHealth:
    def test_health_ok(self, client: TestClient) -> None:
        response = client.get("/health")
        assert response.status_code == 200
        assert response.json() == {"status": "ok"}


class TestReady:
    def test_ready_ok(self, client: TestClient) -> None:
        response = client.get("/ready")
        assert response.status_code == 200
        body = response.json()
        assert body["status"] == "ready"
        assert body["app_name"] == "app"
        assert body["environment"] == "testing"


class TestGreet:
    def test_greet_default(self, client: TestClient) -> None:
        response = client.get("/greet")
        assert response.status_code == 200
        assert response.json()["name"] == "world"

    def test_greet_with_name(self, client: TestClient) -> None:
        response = client.get("/greet", params={"name": "Alice"})
        assert response.status_code == 200
        body = response.json()
        assert body["name"] == "Alice"
        assert body["message"] == "Hello, Alice!"

    def test_greet_missing_name_rejected(self, client: TestClient) -> None:
        response = client.get("/greet", params={"name": ""})
        assert response.status_code == 422


class TestAnalyze:
    def test_analyze_valid(self, client: TestClient) -> None:
        response = client.post("/analyze", json={"text": "hello world\nfoo"})
        assert response.status_code == 200
        body = response.json()
        assert body["characters"] == 15
        assert body["words"] == 3
        assert body["lines"] == 2

    def test_analyze_empty_text_rejected(self, client: TestClient) -> None:
        response = client.post("/analyze", json={"text": ""})
        assert response.status_code == 422

    def test_analyze_missing_field_rejected(self, client: TestClient) -> None:
        response = client.post("/analyze", json={})
        assert response.status_code == 422

    def test_analyze_oversize_rejected(self, client: TestClient) -> None:
        response = client.post("/analyze", json={"text": "x" * 10_001})
        assert response.status_code == 422


class TestStatus:
    def test_status_returns_info(self, client: TestClient) -> None:
        response = client.get("/status")
        assert response.status_code == 200
        body = response.json()
        assert body["app_name"] == "app"
        assert body["environment"] == "testing"
        assert "python_version" in body
        assert "uptime_seconds" in body


class TestRequestSizeLimit:
    def test_oversize_payload_rejected(self, client: TestClient) -> None:
        settings = Settings(_env_file=None, max_request_bytes=10)
        app = create_app(settings)
        with TestClient(app) as small_client:
            response = small_client.post("/analyze", json={"text": "x" * 100})
            assert response.status_code == 413

    def test_small_payload_accepted(self, client: TestClient) -> None:
        response = client.post("/analyze", json={"text": "hi"})
        assert response.status_code == 200


class TestErrorHandling:
    def test_not_found(self, client: TestClient) -> None:
        response = client.get("/nonexistent")
        assert response.status_code == 404

    def test_method_not_allowed(self, client: TestClient) -> None:
        response = client.post("/health")
        assert response.status_code == 405

    def test_request_id_header_present(self, client: TestClient) -> None:
        response = client.get("/health")
        assert "x-request-id" in response.headers
