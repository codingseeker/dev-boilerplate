.PHONY: install dev format lint typecheck test coverage build run serve docker-build docker-run clean help

help:
	@echo "Available targets:"
	@echo "  install       Install runtime dependencies"
	@echo "  dev           Install development dependencies"
	@echo "  format        Format code with ruff"
	@echo "  lint          Lint code with ruff"
	@echo "  typecheck     Run mypy type checking"
	@echo "  test          Run the test suite"
	@echo "  coverage      Run tests with coverage reporting"
	@echo "  build         Build the package"
	@echo "  run           Run the CLI application"
	@echo "  serve         Start the REST API server"
	@echo "  docker-build  Build the Docker image"
	@echo "  docker-run    Run the Docker container"
	@echo "  clean         Remove build artifacts and caches"

install:
	uv sync

dev:
	uv sync --extra dev

format:
	uv run ruff format src tests
	uv run ruff check --fix src tests

lint:
	uv run ruff check src tests
	uv run ruff format --check src tests

typecheck:
	uv run mypy

test:
	uv run pytest

coverage:
	uv run pytest --cov --cov-report=term-missing --cov-report=xml

build:
	uv build

run:
	uv run python -m app run

serve:
	uv run python -m app serve

docker-build:
	docker build -t app .

docker-run:
	docker run --rm -p 8080:8080 app

clean:
	rm -rf build dist *.egg-info .pytest_cache .mypy_cache .ruff_cache .coverage htmlcov
	find . -type d -name __pycache__ -exec rm -rf {} +
