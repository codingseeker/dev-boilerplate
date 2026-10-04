from __future__ import annotations

from typer.testing import CliRunner

from app import __version__
from app.cli.commands import app

runner = CliRunner()


class TestHelp:
    def test_help_lists_commands(self) -> None:
        result = runner.invoke(app, ["--help"])
        assert result.exit_code == 0
        assert "run" in result.output
        assert "serve" in result.output

    def test_help_shows_version_option(self) -> None:
        result = runner.invoke(app, ["--help"])
        assert "--version" in result.output

    def test_no_args_shows_help(self) -> None:
        result = runner.invoke(app, [])
        assert result.exit_code == 2
        assert "run" in result.output
        assert "serve" in result.output


class TestVersion:
    def test_version(self) -> None:
        result = runner.invoke(app, ["--version"])
        assert result.exit_code == 0
        assert __version__ in result.output


class TestRunCommand:
    def test_run_default(self) -> None:
        result = runner.invoke(app, ["run"])
        assert result.exit_code == 0
        assert "Hello, world!" in result.output

    def test_run_with_name(self) -> None:
        result = runner.invoke(app, ["run", "--name", "Alice"])
        assert result.exit_code == 0
        assert "Hello, Alice!" in result.output

    def test_run_short_name_flag(self) -> None:
        result = runner.invoke(app, ["run", "-n", "Bob"])
        assert result.exit_code == 0
        assert "Hello, Bob!" in result.output

    def test_run_reports_status(self) -> None:
        result = runner.invoke(app, ["run"])
        assert "env=development" in result.output

    def test_run_with_env_override(self) -> None:
        result = runner.invoke(app, ["run", "--env", "production"])
        assert result.exit_code == 0
        assert "env=production" in result.output


class TestInvalidArguments:
    def test_invalid_environment_rejected(self) -> None:
        result = runner.invoke(app, ["run", "--env", "staging"])
        assert result.exit_code != 0

    def test_invalid_log_level_rejected(self) -> None:
        result = runner.invoke(app, ["run", "--log-level", "verbose"])
        assert result.exit_code != 0

    def test_unknown_command_rejected(self) -> None:
        result = runner.invoke(app, ["frobnicate"])
        assert result.exit_code != 0
