"""Install a checksummed small coding model and native Apple Silicon llama.cpp."""
import hashlib
import os
from pathlib import Path
import platform
import re
import shlex
import shutil
import tarfile
import time
import urllib.request
from ovx_cli import ROOT, STATE, load_env

MODEL = "Qwen2.5-Coder-3B-Instruct-Q4_K_M.gguf"
MODEL_SHA = "3da3afe6cf5c674ac195803ea0dd6fee7e1c228c2105c1ce8c66890d1d4ab460"
MODEL_URL = "https://huggingface.co/bartowski/Qwen2.5-Coder-3B-Instruct-GGUF/resolve/main/" + MODEL
LLAMA_URL = "https://github.com/ggml-org/llama.cpp/releases/download/b11207/llama-b11207-bin-macos-arm64.tar.gz"
LLAMA_SHA = "0e5ec94359b99707453f76d735988cd53b5f43c606c4974f4e2a580501a087f0"


def digest(path):
    result = hashlib.sha256()
    with path.open("rb") as source:
        for chunk in iter(lambda: source.read(1024 * 1024), b""):
            result.update(chunk)
    return result.hexdigest()


def download(url, path, expected, size):
    path.parent.mkdir(parents=True, exist_ok=True)
    if path.exists():
        if digest(path) != expected:
            raise RuntimeError(f"Existing file checksum differs: {path}; move it aside before retrying.")
        return
    if shutil.disk_usage(path.parent).free < size + 512 * 1024 * 1024:
        raise RuntimeError(f"Not enough disk space to download {path.name}.")
    partial = path.with_suffix(".part")
    total = 0
    last = time.monotonic()
    with urllib.request.urlopen(url, timeout=120) as response, partial.open("wb") as output:
        while chunk := response.read(1024 * 1024):
            output.write(chunk)
            total += len(chunk)
            if time.monotonic() - last > 10:
                print(f"{path.name}: {total / 1e6:.0f} MB downloaded", flush=True)
                last = time.monotonic()
    if digest(partial) != expected:
        raise RuntimeError(f"Download checksum failed: {partial}")
    partial.rename(path)


def configure(values):
    path = ROOT / ".env"
    text = path.read_text()
    for key, value in values.items():
        line = key + "=" + shlex.quote(str(value))
        pattern = r"^" + re.escape(key) + r"=.*$"
        text = re.sub(pattern, lambda _: line, text, flags=re.M) if re.search(pattern, text, re.M) else text + "\n" + line + "\n"
    path.write_text(text)
    os.chmod(path, 0o600)


def install():
    env = load_env()
    if platform.system() == "Darwin" and platform.machine() == "arm64":
        directory = STATE / "tools/llama"
        server = directory / "llama-b11207/llama-server"
        if not server.exists():
            archive = STATE / "tools/llama-b11207.tar.gz"
            download(LLAMA_URL, archive, LLAMA_SHA, 12_000_000)
            with tarfile.open(archive) as source:
                source.extractall(directory, filter="data")
            archive.unlink()
        link = directory / "bin"
        if not link.exists():
            link.symlink_to("llama-b11207", target_is_directory=True)
    elif not shutil.which("llama-server", path=env["PATH"]):
        raise RuntimeError("Install llama-server for your OS first; then rerun setup-model.")
    target = STATE / "models" / MODEL
    download(MODEL_URL, target, MODEL_SHA, 1_929_903_360)
    configure({"MODEL_PROVIDER": "openai_compatible", "MODEL_PATH": target,
               "MODEL_BASE_URL": "http://127.0.0.1:8080/v1", "MODEL_NAME": "ovx-local",
               "MODEL_JSON_MODE": "true", "MODEL_CONTEXT_SIZE": "8192",
               "MODEL_CONTEXT_BYTES": "18000", "MODEL_MAX_TOKENS": "1800"})
    print("Local coding model verified and configured. Run bin/ovx restart.")
