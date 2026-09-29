"""Prepare a dependency image for one explicitly selected Rails application."""
import argparse
import fcntl
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import tempfile
from ovx_cli import ROOT, load_env


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('repository', type=Path)
    parser.add_argument('--cwd', default='.')
    args = parser.parse_args()
    repository = args.repository.resolve(strict=True)
    app = (repository / args.cwd).resolve(strict=True)
    relative = app.relative_to(repository).as_posix()
    ruby = (app / '.ruby-version').read_text().strip().removeprefix('ruby-')
    if not re.fullmatch(r'\d+\.\d+\.\d+', ruby):
        raise SystemExit('Expected a numeric Ruby version in .ruby-version')
    env = load_env()
    tag = 'ovx-project-' + hashlib.sha256(str(app).encode()).hexdigest()[:16]
    with tempfile.TemporaryDirectory(prefix='ovx-rails-build-') as directory:
        context = Path(directory)
        for name in ('Gemfile', 'Gemfile.lock'):
            shutil.copyfile(app / name, context / name)
        shutil.copyfile(ROOT / 'docker/sandbox-project-rails.Dockerfile', context / 'Dockerfile')
        shutil.copyfile(ROOT / 'docker/rails-test-entrypoint.sh', context / 'entrypoint.sh')
        subprocess.run(['docker', 'build', '--build-arg', 'RUBY_VERSION=' + ruby,
                        '--build-arg', 'LOCAL_UID=' + str(os.getuid()),
                        '--build-arg', 'LOCAL_GID=' + str(os.getgid()), '-t', tag, str(context)], env=env, check=True)
    registry = Path(env['OVX_WORKSPACE_ROOT']) / '.ovx' / 'sandbox-images.json'
    registry.parent.mkdir(parents=True, exist_ok=True)
    with registry.with_suffix('.lock').open('a') as lock:
        fcntl.flock(lock, fcntl.LOCK_EX)
        profiles = json.loads(registry.read_text()) if registry.exists() else {}
        profiles[str(repository) + '::' + relative] = tag
        profiles[str(app) + '::.'] = tag
        temporary = registry.with_suffix('.tmp')
        temporary.write_text(json.dumps(profiles, indent=2))
        temporary.chmod(0o600)
        temporary.replace(registry)
    print(f'Prepared {tag} for {repository} (cwd={relative}). New test runs use it automatically.')


if __name__ == '__main__':
    main()
