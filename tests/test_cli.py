import importlib.util
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location("ovx_cli", Path(__file__).resolve().parents[1] / "bin/ovx_cli.py")
cli = importlib.util.module_from_spec(spec)
spec.loader.exec_module(cli)


class ConfigTest(unittest.TestCase):
    def setUp(self):
        clean = {key: value for key, value in os.environ.items()
                 if not key.startswith(("OVX_", "MODEL_", "SECRET_KEY_BASE"))}
        environment = patch.dict(os.environ, clean, clear=True)
        environment.start()
        self.addCleanup(environment.stop)

    def test_configuration_is_data_and_expands_paths(self):
        with tempfile.TemporaryDirectory() as directory:
            envfile = Path(directory) / ".env"
            envfile.write_text('OVX_INSTALL_ROOT="/tmp/my install"\nOVX_WORKSPACE_ROOT="${OVX_INSTALL_ROOT}/workspaces"\nVALUE="$(touch /tmp/ovx-must-never-execute)"\n')
            env = cli.load_env(envfile)
            self.assertEqual(env["OVX_WORKSPACE_ROOT"], "/tmp/my install/workspaces")
            self.assertEqual(env["VALUE"], "$(touch /tmp/ovx-must-never-execute)")

    def test_explicit_environment_override(self):
        with tempfile.TemporaryDirectory() as directory:
            envfile = Path(directory) / ".env"
            envfile.write_text("MODEL_PROVIDER=mock\n")
            with patch.dict(os.environ, {"MODEL_PROVIDER": "openai_compatible"}):
                self.assertEqual(cli.load_env(envfile)["MODEL_PROVIDER"], "openai_compatible")

    def test_invalid_config_fails_with_line_number(self):
        with tempfile.TemporaryDirectory() as directory:
            envfile = Path(directory) / ".env"
            envfile.write_text("some command\n")
            with self.assertRaisesRegex(RuntimeError, ":1:"):
                cli.load_env(envfile)

    def test_initialize_preserves_existing_config_and_creates_private_tokens(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / ".env.example").write_text("OVX_API_TOKEN=\nOVX_RUNTIME_TOKEN=\nSECRET_KEY_BASE=\n")
            with patch.object(cli, "ROOT", root), patch.object(cli, "STATE", root / ".ovx"):
                cli.initialize()
                env = cli.load_env(root / ".env")
                self.assertEqual(len(env["OVX_API_TOKEN"]), 64)
                self.assertNotEqual(env["OVX_API_TOKEN"], env["OVX_RUNTIME_TOKEN"])
                self.assertEqual((root / ".env").stat().st_mode & 0o777, 0o600)
                original = (root / ".env").read_bytes()
                cli.initialize()
                self.assertEqual((root / ".env").read_bytes(), original)


class ProcessTest(unittest.TestCase):
    def test_running_identity_does_not_depend_on_scheduler_state(self):
        responses = [subprocess.CompletedProcess([], 0, "Sun Sep 27 12:00:00 2026 S\n"),
                     subprocess.CompletedProcess([], 0, "Sun Sep 27 12:00:00 2026 R\n")]
        with patch.object(subprocess, "run", side_effect=responses):
            self.assertEqual(cli.identity(123), cli.identity(123))

    def test_reused_pid_is_not_stopped(self):
        with tempfile.TemporaryDirectory() as directory, patch.object(cli, "STATE", Path(directory)):
            (Path(directory) / "run").mkdir()
            record = Path(directory) / "run/worker.json"
            record.write_text(json.dumps({"pid": 123, "identity": "old process"}))
            with patch.object(cli, "identity", return_value="new process"), patch.object(os, "killpg") as kill:
                cli.stop_process("worker")
                kill.assert_not_called()
            self.assertFalse(record.exists())

    def test_existing_demo_is_never_deleted(self):
        with tempfile.TemporaryDirectory() as directory:
            existing = Path(directory) / "demo-ruby"
            existing.mkdir()
            (existing / "important.txt").write_text("user work")
            env = cli.load_env()
            env["OVX_WORKSPACE_ROOT"] = directory
            created = cli.demo(env)
            self.assertNotEqual(created, existing)
            self.assertEqual((existing / "important.txt").read_text(), "user work")
            self.assertTrue((created / ".git").is_dir())


if __name__ == "__main__":
    unittest.main()
