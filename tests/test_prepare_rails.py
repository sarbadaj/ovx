import json
from pathlib import Path
import sys
import tempfile
import unittest
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / 'bin'))
import ovx_prepare_rails as prepare


class PrepareRailsTest(unittest.TestCase):
    def test_only_dependency_files_are_uploaded_and_both_folder_forms_registered(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory).resolve()
            app = root / 'project' / 'server'
            app.mkdir(parents=True)
            for name, value in {'.ruby-version': 'ruby-3.4.2', 'Gemfile': "gem 'pg'", 'Gemfile.lock': '', '.env': 'SECRET=private'}.items():
                (app / name).write_text(value)
            env = {'OVX_WORKSPACE_ROOT': str(root / 'workspaces')}
            def build(command, **kwargs):
                context = Path(command[-1])
                self.assertEqual({'Gemfile', 'Gemfile.lock', 'Dockerfile', 'entrypoint.sh'}, {p.name for p in context.iterdir()})
                self.assertIn('RUBY_VERSION=3.4.2', command)
                self.assertIn('OVX_DB_KIND=postgresql', command)
            with patch.object(sys, 'argv', ['prepare', str(app.parent), '--cwd', 'server']), patch.object(prepare, 'load_env', return_value=env), patch.object(prepare.subprocess, 'run', side_effect=build):
                prepare.main()
            registry = json.loads((root / 'workspaces/.ovx/sandbox-images.json').read_text())
            self.assertEqual(registry[str(app.parent) + '::server'], registry[str(app) + '::.'])

    def test_application_cannot_escape_selected_repository(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory).resolve()
            (root / 'project').mkdir()
            with patch.object(sys, 'argv', ['prepare', str(root / 'project'), '--cwd', '..']), patch.object(prepare.subprocess, 'run') as build:
                with self.assertRaises(ValueError):
                    prepare.main()
                build.assert_not_called()

    def test_ruby_version_can_come_from_lockfile(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory).resolve()
            app = root / 'project'
            app.mkdir()
            (app / 'Gemfile').write_text("gem 'sqlite3'")
            (app / 'Gemfile.lock').write_text('RUBY VERSION\n   ruby 3.3.7\n')
            env = {'OVX_WORKSPACE_ROOT': str(root / 'workspaces')}
            def build(command, **_):
                self.assertIn('RUBY_VERSION=3.3.7', command)
                self.assertIn('OVX_DB_KIND=sqlite', command)
            with patch.object(sys, 'argv', ['prepare', str(app)]), patch.object(prepare, 'load_env', return_value=env), patch.object(prepare.subprocess, 'run', side_effect=build):
                prepare.main()
