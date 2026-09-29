"""Standard-library CLI and process manager for the local OVX installation."""
import argparse
import fcntl
import json
import os
import tempfile
from pathlib import Path
import re
import secrets
import shlex
import shutil
import signal
import subprocess
import sys
import time
import urllib.error
import urllib.parse
import urllib.request

ROOT = Path(__file__).resolve().parent.parent
STATE = ROOT / ".ovx"


def load_env(path=None):
    env = dict(os.environ, OVX_INSTALL_ROOT=str(ROOT))
    configured = {}
    path = Path(path or ROOT / ".env")
    if path.exists():
        for number, line in enumerate(path.read_text().splitlines(), 1):
            line = line.strip()
            if not line or line.startswith("#"):
                continue
            match = re.fullmatch(r"(?:export\s+)?([A-Za-z_][A-Za-z0-9_]*)=(.*)", line)
            if not match:
                raise RuntimeError(f"{path}:{number}: expected KEY=value")
            key, value = match.groups()
            words = shlex.split(value, comments=True)
            if len(words) > 1:
                raise RuntimeError(f"{path}:{number}: quote values containing spaces")
            value = words[0] if words else ""
            scope = {**env, **configured}
            value = re.sub(r"\$\{([A-Za-z_][A-Za-z0-9_]*)\}|\$([A-Za-z_][A-Za-z0-9_]*)",
                           lambda m: scope.get(m[1] or m[2], ""), value)
            configured[key] = os.path.expanduser(value)
    env = {**env, **configured, **os.environ, "OVX_INSTALL_ROOT": str(ROOT)}
    # Keep packaged toolchains and Homebrew Ruby ahead of stale system/RVM shims.
    paths = [STATE / "tools/go/bin", STATE / "tools/llama/bin"]
    if Path("/opt/homebrew/opt/ruby/bin/ruby").exists():
        paths += [Path("/opt/homebrew/opt/ruby/bin"), Path("/opt/homebrew/bin")]
        env.pop("GEM_HOME", None)
        env.pop("GEM_PATH", None)
    env["PATH"] = os.pathsep.join(map(str, paths)) + os.pathsep + env.get("PATH", "")
    env.setdefault("OVX_WORKSPACE_ROOT", str(STATE / "workspaces"))
    env.setdefault("BUNDLE_PATH", str(ROOT / "apps/api/vendor/bundle"))
    env.setdefault("BUNDLE_APP_CONFIG", str(ROOT / "apps/api/.bundle"))
    env.setdefault("GOCACHE", str(STATE / "cache/go"))
    env.setdefault("GOMODCACHE", str(STATE / "cache/gomod"))
    return env


def initialize():
    path = ROOT / ".env"
    if not path.exists():
        content = (ROOT / ".env.example").read_text().replace("${OVX_INSTALL_ROOT}", str(ROOT))
        content = re.sub(r"^OVX_WORKSPACE_ROOT=(.*)$", lambda m: "OVX_WORKSPACE_ROOT=" + shlex.quote(m[1]), content, flags=re.M)
        for name in ("OVX_API_TOKEN", "OVX_RUNTIME_TOKEN", "SECRET_KEY_BASE"):
            content = content.replace(f"{name}=\n", f"{name}={secrets.token_hex(32)}\n")
        fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
        with os.fdopen(fd, "w") as handle:
            handle.write(content)
        print(f"Created {path} with private local tokens.", file=sys.stderr, flush=True)
    for directory in ("run", "logs"):
        (STATE / directory).mkdir(parents=True, exist_ok=True)


def run(args, env, cwd=ROOT, **kwargs):
    return subprocess.run([str(a) for a in args], cwd=cwd, env=env, check=True, **kwargs)


def compose(env, *args):
    return run(["docker", "compose", "--project-name", "ovx-local", "--file", ROOT / "docker-compose.yml", *args], env)


def ensure_docker(env):
    def available():
        try:
            return subprocess.run(["docker", "info"], env=env, stdout=subprocess.DEVNULL,
                                  stderr=subprocess.DEVNULL, timeout=5).returncode == 0
        except (OSError, subprocess.TimeoutExpired):
            return False
    if available():
        return
    if sys.platform == "darwin":
        print("Starting Docker Desktop for OVX...", flush=True)
        subprocess.run(["open", "-a", "Docker"], env=env, check=True)
        deadline = time.monotonic() + 120
        while time.monotonic() < deadline:
            if available():
                return
            time.sleep(2)
    raise RuntimeError("Docker is unavailable. Start Docker Desktop or Docker Engine, then retry bin/ovx start.")


def request(env, path, data=None, method=None, base=None, token=None, timeout=30):
    url = (base or env.get("OVX_API_URL", "http://127.0.0.1:3000")).rstrip("/") + path
    headers = {"Accept": "application/json"}
    bearer = env.get("OVX_API_TOKEN", "") if token is None else token
    if bearer:
        headers["Authorization"] = "Bearer " + bearer
    body = None if data is None else json.dumps(data).encode()
    if body is not None:
        headers["Content-Type"] = "application/json"
    req = urllib.request.Request(url, body, headers, method=method)
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
    try:
        with opener.open(req, timeout=timeout) as response:
            return json.load(response)
    except urllib.error.HTTPError as error:
        try:
            detail = json.loads(error.read()).get("error", str(error))
        except (ValueError, AttributeError):
            detail = str(error)
        raise RuntimeError(f"HTTP {error.code}: {detail}") from error
    except (urllib.error.URLError, TimeoutError) as error:
        raise RuntimeError(f"Cannot connect to {url}: {error}") from error


def identity(pid):
    try:
        # Start time is stable; process status is not (running/sleeping changes).
        result = subprocess.run(["ps", "-p", str(pid), "-o", "lstart=", "-o", "stat="], capture_output=True, text=True, check=True)
        parts = result.stdout.strip().split()
        return " ".join(parts[:-1]) if parts and not parts[-1].startswith("Z") else ""
    except (subprocess.CalledProcessError, OSError):
        return ""


def alive(record):
    return bool(record.get("identity")) and identity(record.get("pid", 0)) == record["identity"]


def record_for(name):
    try:
        return json.loads((STATE / "run" / (name + ".json")).read_text())
    except (OSError, ValueError):
        return {}


def start_process(name, args, env, cwd=ROOT):
    if alive(record_for(name)):
        print(f"{name:14} already running", flush=True)
        return False
    log = STATE / "logs" / (name + ".log")
    with log.open("ab") as output:
        proc = subprocess.Popen([str(a) for a in args], cwd=cwd, env=env,
                                stdin=subprocess.DEVNULL, stdout=output, stderr=output, start_new_session=True)
    time.sleep(0.2)
    if proc.poll() is not None:
        raise RuntimeError(f"{name} exited; inspect {log}")
    (STATE / "run" / (name + ".json")).write_text(json.dumps({"pid": proc.pid, "identity": identity(proc.pid)}))
    print(f"{name:14} starting (PID {proc.pid})", flush=True)
    return True


def stop_process(name):
    record = record_for(name)
    if alive(record):
        os.killpg(record["pid"], signal.SIGTERM)
        deadline = time.monotonic() + 12
        while alive(record) and time.monotonic() < deadline:
            time.sleep(0.2)
        if alive(record):
            os.killpg(record["pid"], signal.SIGKILL)
        print(f"{name:14} stopped")
    (STATE / "run" / (name + ".json")).unlink(missing_ok=True)


def wait_ready(env, url, path="/health", timeout=60, name=None, token=None):
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        if name and not alive(record_for(name)):
            raise RuntimeError(f"{name} exited; run bin/ovx logs {name}")
        try:
            return request(env, path, base=url, token=token, timeout=2)
        except RuntimeError:
            time.sleep(0.5)
    raise RuntimeError(f"Timed out waiting for {url}{path}; inspect bin/ovx logs")


def doctor(env):
    missing = False
    for command in ("git", "docker", "ruby", "bundle", "go", "rg", "node", "npm"):
        found = shutil.which(command, path=env["PATH"])
        print(f"{command:14} {found or 'MISSING'}")
        missing |= found is None
    try:
        run(["ruby", "-e", 'abort "Ruby 3.2+ required" if Gem::Version.new(RUBY_VERSION) < Gem::Version.new("3.2")'], env, capture_output=True)
        run(["docker", "info"], env, capture_output=True)
        run(["docker", "compose", "version"], env, capture_output=True)
        print("Docker engine  ready")
    except (subprocess.CalledProcessError, OSError):
        print("Ruby/Docker    unavailable; use Ruby 3.2+ and start Docker Desktop/Engine")
        missing = True
    if env.get("MODEL_PROVIDER", "mock") == "mock":
        print("Local model    mock (calculator demo only)")
    else:
        try:
            request(env, "/models", base=env.get("MODEL_BASE_URL", "http://127.0.0.1:8080/v1"), token=env.get("MODEL_API_KEY", ""), timeout=3)
            print("Local model    reachable")
        except RuntimeError:
            print("Local model    not reachable; configure MODEL_PATH or start your model server")
    return not missing


def setup(env):
    ensure_docker(env)
    if not doctor(env):
        raise RuntimeError("Install missing prerequisites, then retry bin/ovx setup.")
    Path(env["OVX_WORKSPACE_ROOT"]).mkdir(parents=True, exist_ok=True)
    compose(env, "up", "-d", "--wait", "postgres", "redis")
    run(["docker", "build", "-t", "ovx-sandbox-ruby:latest", "-f", "docker/sandbox-ruby.Dockerfile", "."], env)
    run(["docker", "build", "-t", "ovx-sandbox-demo-rails:latest", "-f", "docker/sandbox-demo-rails.Dockerfile", "."], env)
    run(["bundle", "install"], env, ROOT / "apps/api")
    run(["bundle", "exec", "rails", "db:prepare"], env, ROOT / "apps/api")
    build_runtime(env)
    print("Setup complete. Run bin/ovx start, then bin/ovx smoke.")


def build_runtime(env):
    (STATE / "bin").mkdir(parents=True, exist_ok=True)
    run(["go", "build", "-o", STATE / "bin/ovx-runtime", "."], env, ROOT / "services/runtime")


def start(env):
    for key in ("OVX_API_TOKEN", "OVX_RUNTIME_TOKEN"):
        if len(env.get(key, "")) < 24 or env[key].startswith("change-"):
            raise RuntimeError(f"Set a random {key} of at least 24 characters in .env.")
    api = env.get("OVX_API_URL", "http://127.0.0.1:3000")
    parsed = urllib.parse.urlparse(api)
    if parsed.scheme != "http" or parsed.hostname not in ("127.0.0.1", "localhost") or parsed.path not in ("", "/"):
        raise RuntimeError("OVX_API_URL must be a loopback HTTP URL without a path.")
    Path(env["OVX_WORKSPACE_ROOT"]).mkdir(parents=True, exist_ok=True)
    ensure_docker(env)
    compose(env, "up", "-d", "--wait", "postgres", "redis")
    run(["docker", "image", "inspect", env.get("OVX_SANDBOX_IMAGE", "ovx-sandbox-ruby:latest")], env, stdout=subprocess.DEVNULL)
    run(["bundle", "exec", "rails", "db:prepare"], env, ROOT / "apps/api")
    created = []
    try:
        if env.get("MODEL_PROVIDER", "mock") != "mock":
            base = env.get("MODEL_BASE_URL", "http://127.0.0.1:8080/v1")
            model_uri = urllib.parse.urlparse(base)
            if env.get("MODEL_PATH"):
                model_path = Path(env["MODEL_PATH"])
                if not model_path.is_file():
                    raise RuntimeError("MODEL_PATH must point to an existing GGUF file.")
                if model_uri.hostname not in ("127.0.0.1", "localhost") or model_uri.scheme != "http":
                    raise RuntimeError("Managed llama-server requires a local HTTP MODEL_BASE_URL.")
                args = ["llama-server", "-m", model_path, "--host", "127.0.0.1", "--port", str(model_uri.port or 8080),
                        "--alias", env.get("MODEL_NAME", "ovx-local"), "--ctx-size", env.get("MODEL_CONTEXT_SIZE", "8192"), "--parallel", "1"]
                if start_process("model", args, env):
                    created.append("model")
            wait_ready(env, base, "/models", timeout=180, name="model" if env.get("MODEL_PATH") else None, token=env.get("MODEL_API_KEY", ""))
        if not alive(record_for("runtime")):
            build_runtime(env)
        if start_process("runtime", [STATE / "bin/ovx-runtime"], env):
            created.append("runtime")
        wait_ready(env, env.get("OVX_RUNTIME_URL", "http://127.0.0.1:7070"), name="runtime")
        if start_process("api", ["bundle", "exec", "rails", "server", "-b", "127.0.0.1", "-p", str(parsed.port or 3000), "-P", STATE / "run/rails.pid"], env, ROOT / "apps/api"):
            created.append("api")
        wait_ready(env, api, name="api")
        request(env, "/api/repositories")
        if start_process("worker", ["bundle", "exec", "sidekiq", "-C", "config/sidekiq.yml"], env, ROOT / "apps/api"):
            created.append("worker")
        time.sleep(1)
        if not alive(record_for("worker")):
            raise RuntimeError("Worker exited; inspect bin/ovx logs worker")
    except BaseException:
        for name in reversed(created):
            stop_process(name)
        raise
    status(env)


def status(env):
    healthy = True
    for label, url in (("OVX Agent", env.get("OVX_API_URL", "http://127.0.0.1:3000")), ("Runtime", env.get("OVX_RUNTIME_URL", "http://127.0.0.1:7070"))):
        try:
            request(env, "/health", base=url, timeout=2)
            if label == "OVX Agent":
                # /health can return 200 while PostgreSQL or Redis has stopped.
                request(env, "/api/repositories", base=url, timeout=3)
            state = "running"
        except RuntimeError:
            state = "stopped/unreachable or database offline"
            healthy = False
        print(f"{label:14} {state}")
    worker = alive(record_for("worker"))
    print(f"{'Worker':14} {'running' if worker else 'stopped'}")
    if env.get("MODEL_PROVIDER", "mock") == "mock":
        print(f"{'Local Model':14} mock — calculator demo only")
    else:
        try:
            request(env, "/models", base=env.get("MODEL_BASE_URL"), token=env.get("MODEL_API_KEY", ""), timeout=2)
            print(f"{'Local Model':14} running ({env.get('MODEL_NAME')})")
        except RuntimeError:
            print(f"{'Local Model':14} unavailable")
            healthy = False
    print(f"{'VS Code':14} open OVX sidebar → Connect")
    return healthy and worker


def demo(env, fixture="demo-ruby"):
    import tempfile
    workspace = Path(env["OVX_WORKSPACE_ROOT"])
    workspace.mkdir(parents=True, exist_ok=True)
    target = Path(tempfile.mkdtemp(prefix=fixture + "-", dir=workspace))
    shutil.copytree(ROOT / "examples" / fixture, target, dirs_exist_ok=True)
    run(["git", "init", "-q"], env, target)
    run(["git", "add", "."], env, target)
    run(["git", "-c", "user.name=OVX Demo", "-c", "user.email=ovx@example.local", "commit", "-qm", "demo baseline"], env, target)
    return target


def wait_task(env, task_id, timeout=600):
    deadline = time.monotonic() + timeout
    previous = None
    while time.monotonic() < deadline:
        task = request(env, f"/api/tasks/{task_id}")
        if task["state"] != previous:
            print(f"task {task_id}: {task['state']}", file=sys.stderr, flush=True)
            previous = task["state"]
        if task["state"] in ("completed", "failed", "cancelled", "awaiting_approval", "recovery_required"):
            return task
        time.sleep(1)
    raise RuntimeError(f"Timed out waiting for task {task_id}; inspect its saved history.")


def smoke(env):
    if env.get("MODEL_PROVIDER", "mock") != "mock":
        raise RuntimeError("Smoke requires MODEL_PROVIDER=mock. Use a normal task to validate your real model.")
    target = demo(env)
    repository = request(env, "/api/repositories", {"path": str(target)})
    task = request(env, "/api/tasks", {"repository_id": repository["id"], "request": "Fix the failing calculator test. Do not modify tests. Run relevant tests and inspect git diff."})
    task = wait_task(env, task["id"], 180)
    if task["state"] != "completed":
        raise RuntimeError(f"Smoke task failed: {task.get('error_message') or task['state']}")
    review = request(env, f"/api/tasks/{task['id']}/diff")
    if not review.get("files"):
        raise RuntimeError("Smoke produced no reviewable diff.")
    if "a - b" not in (target / "lib/calculator.rb").read_text():
        raise RuntimeError("Source was changed before acceptance!")
    result = request(env, f"/api/tasks/{task['id']}/apply_result", {"expected_patch_sha256": review["patch_sha256"]})
    if result.get("resolution") != "accepted" or "a + b" not in (target / "lib/calculator.rb").read_text():
        raise RuntimeError("Accepted patch was not applied.")
    if (target / "test/calculator_test.rb").read_bytes() != (ROOT / "examples/demo-ruby/test/calculator_test.rb").read_bytes():
        raise RuntimeError("The fixture test was modified.")
    print(f"OVX MOCK SMOKE TEST PASSED\nTask: {task['id']}\nSource: {target}")


def run_rspec(env, args):
    if len(args) not in (1, 3) or (len(args) == 3 and args[1] != "--cwd"):
        raise RuntimeError("Usage: bin/ovx run-rspec /absolute/project [--cwd server]")
    repository = Path(args[0]).expanduser().resolve(strict=True)
    app = (repository / (args[2] if len(args) == 3 else ".")).resolve(strict=True)
    relative = app.relative_to(repository)
    if not app.is_dir() or not (app / "Gemfile").is_file() or not (app / "spec").is_dir():
        raise RuntimeError("RSpec application must contain Gemfile and spec")
    run(["python3", ROOT / "bin/ovx_prepare_rails.py", repository, "--cwd", str(relative)], env)
    registry = Path(env["OVX_WORKSPACE_ROOT"]) / ".ovx/sandbox-images.json"
    image = json.loads(registry.read_text())[str(repository) + "::" + relative.as_posix()]
    workspace_root = Path(env["OVX_WORKSPACE_ROOT"])
    workspace_root.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix="rspec-", dir=workspace_root) as directory:
        snapshot = Path(directory) / "project"
        shutil.copytree(app, snapshot, symlinks=True,
                        ignore=shutil.ignore_patterns(".git", ".env", ".env.*", "node_modules", "vendor", "tmp", "log", "coverage", "storage", ".bundle", ".ovx"))
        print(f"Running full RSpec for {app} in an isolated OVX sandbox", flush=True)
        command = ["docker", "run", "--rm", "--pull=never", "--network", "none", "--cpus", "2", "--memory", "2g", "--pids-limit", "256",
                   "--read-only", "--cap-drop", "ALL", "--security-opt", "no-new-privileges:true", "--user", f"{os.getuid()}:{os.getgid()}",
                   "--tmpfs", "/tmp:rw,noexec,nosuid,size=512m", "-e", "HOME=/tmp", "-e", "RAILS_ENV=test", "-e", "RACK_ENV=test",
                   "--mount", f"type=bind,src={snapshot},dst=/workspace", "-w", "/workspace", image, "bundle", "exec", "rspec"]
        completed = subprocess.run(command, env=env)
        if completed.returncode:
            raise SystemExit(completed.returncode)


def dispatch(command, args, env):
    result = None
    if command == "init":
        return
    if command == "setup":
        setup(env)
    elif command == "setup-model":
        from ovx_model import install
        install()
    elif command in ("start", "restart"):
        if command == "restart":
            for name in ("worker", "api", "runtime", "model"):
                stop_process(name)
        start(env)
    elif command == "stop":
        for name in ("worker", "api", "runtime", "model"):
            stop_process(name)
        compose(env, "stop")
    elif command == "status":
        sys.exit(0 if status(env) else 1)
    elif command == "doctor":
        sys.exit(0 if doctor(env) else 1)
    elif command == "logs":
        for name in args or ["api", "worker", "runtime", "model"]:
            if name not in ("api", "worker", "runtime", "model"):
                raise RuntimeError("Log name must be api, worker, runtime or model")
            path = STATE / "logs" / (name + ".log")
            print(f"\n{name}: {path}")
            if path.exists():
                print("\n".join(path.read_text(errors="replace").splitlines()[-80:]))
    elif command == "demo":
        fixture = "demo-rails" if args == ["rails"] else "demo-ruby"
        print(demo(env, fixture))
    elif command == "smoke":
        smoke(env)
    elif command == "run-rspec":
        run_rspec(env, args)
    elif command == "test":
        run(["go", "test", "./..."], env, ROOT / "services/runtime")
        run(["python3", "-m", "unittest", "discover", "-s", "tests", "-v"], env)
        run(["npm", "test"], env, ROOT / "apps/vscode")
        test_env = {**env, "RAILS_ENV": "test", "DATABASE_URL": env["TEST_DATABASE_URL"]}
        run(["bundle", "exec", "rails", "db:prepare"], test_env, ROOT / "apps/api")
        run(["bundle", "exec", "rails", "test"], test_env, ROOT / "apps/api")
    elif command in ("package-extension", "install-extension"):
        run(["npm", "run", "package"], env, ROOT / "apps/vscode")
        if command == "install-extension":
            packages = list((ROOT / "apps/vscode/dist").glob("*.vsix"))
            if not packages:
                raise RuntimeError("Extension package not found")
            run(["code", "--install-extension", max(packages, key=lambda p: p.stat().st_mtime), "--force"], env)
    elif command == "repos":
        result = request(env, "/api/repositories")
    elif command == "add-repo":
        result = request(env, "/api/repositories", {"path": str(Path(args[0]).expanduser().resolve())})
    elif command == "task":
        if len(args) < 2:
            raise RuntimeError('Usage: bin/ovx task REPO_ID "Fix this failing spec"')
        result = request(env, "/api/tasks", {"repository_id": int(args[0]), "request": " ".join(args[1:])})
    elif command == "approvals":
        result = request(env, "/api/approvals?" + urllib.parse.urlencode({"status": args[0] if args else "pending"}))
    elif command in ("approve", "deny"):
        result = request(env, f"/api/approvals/{int(args[0])}/{command}", {})
    else:
        task_id = int(args[0])
        if command == "wait":
            result = wait_task(env, task_id)
        elif command == "workspace":
            print(request(env, f"/api/tasks/{task_id}").get("workspace_path") or "")
        elif command in ("show", "events", "diff"):
            result = request(env, f"/api/tasks/{task_id}" + ("" if command == "show" else "/" + command))
        else:
            action = {"apply": "apply_result", "reject": "reject_result"}.get(command, command)
            payload = {}
            if command == "apply":
                review = request(env, f"/api/tasks/{task_id}/diff")
                payload["expected_patch_sha256"] = review["patch_sha256"]
            result = request(env, f"/api/tasks/{task_id}/{action}", payload)
    if result is not None:
        print(json.dumps(result, indent=2, ensure_ascii=False))


def main(argv=None):
    parser = argparse.ArgumentParser(description="OVX local coding agent and VS Code service")
    parser.add_argument("command", choices=["init", "setup", "setup-model", "start", "stop", "restart", "status", "doctor", "logs", "repos", "add-repo", "task", "show", "events", "diff", "wait", "approvals", "approve", "deny", "cancel", "retry", "workspace", "apply", "reject", "demo", "smoke", "run-rspec", "test", "package-extension", "install-extension"])
    parser.add_argument("args", nargs=argparse.REMAINDER)
    options = parser.parse_args(argv)
    try:
        initialize()
        env = load_env()
        if options.command in ("setup", "start", "stop", "restart"):
            with (STATE / "run/manager.lock").open("w") as lock:
                try:
                    fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
                except BlockingIOError as error:
                    raise RuntimeError("Another OVX service operation is in progress.") from error
                dispatch(options.command, options.args, env)
        else:
            dispatch(options.command, options.args, env)
    except (RuntimeError, OSError, ValueError, IndexError, subprocess.CalledProcessError) as error:
        print(f"OVX: {error}", file=sys.stderr)
        sys.exit(1)
    except KeyboardInterrupt:
        print("Interrupted.", file=sys.stderr)
        sys.exit(130)
