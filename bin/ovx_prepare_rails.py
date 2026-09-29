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
    version_file = app / '.ruby-version'
    if version_file.is_file():
        ruby = version_file.read_text().strip().removeprefix('ruby-')
    else:
        lock = (app / 'Gemfile.lock').read_text()
        match = re.search(r'^RUBY VERSION\s*\n\s*ruby (\d+\.\d+\.\d+)\b', lock, re.M)
        ruby = match.group(1) if match else '3.4.2'
    if not re.fullmatch(r'\d+\.\d+\.\d+', ruby):
        raise SystemExit('Expected a numeric Ruby version in .ruby-version')
    gemfile = (app / 'Gemfile').read_text()
    if re.search(r'^\s*gem\s+["\']sqlite3["\']', gemfile, re.M):
        database = 'sqlite'
    elif re.search(r'^\s*gem\s+["\']pg["\']', gemfile, re.M):
        database = 'postgresql'
    else:
        raise SystemExit('Rails setup supports sqlite3 or pg. Configure a project sandbox for this database.')
    env = load_env()
    tag = 'ovx-project-' + hashlib.sha256(str(app).encode()).hexdigest()[:16]
    with tempfile.TemporaryDirectory(prefix='ovx-rails-build-') as directory:
        context = Path(directory)
        for name in ('Gemfile', 'Gemfile.lock'):
            shutil.copyfile(app / name, context / name)
        shutil.copyfile(ROOT / 'docker/sandbox-project-rails.Dockerfile', context / 'Dockerfile')
        shutil.copyfile(ROOT / 'docker/rails-test-entrypoint.sh', context / 'entrypoint.sh')
        subprocess.run(['docker', 'build', '--build-arg', 'RUBY_VERSION=' + ruby,
                        '--build-arg', 'OVX_DB_KIND=' + database,
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
