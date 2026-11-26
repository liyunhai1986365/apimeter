"""Disposable loopback-only MySQL and Redis for the billing HTTP suite."""

import json
import os
import secrets
import socket
import subprocess
import tempfile
import time
from contextlib import contextmanager
from dataclasses import dataclass
from pathlib import Path
from urllib.parse import urlsplit


ROOT = Path(__file__).resolve().parents[2]


def _run(command, *, timeout=20, **kwargs):
    try:
        result = subprocess.run(command, text=True, capture_output=True,
                                timeout=timeout, **kwargs)
    except (OSError, subprocess.TimeoutExpired) as exc:
        raise RuntimeError(f"Cannot run {command[0]}: {exc}") from exc
    if result.returncode:
        raise RuntimeError(f"{command[0]} failed: {result.stderr.strip()}")
    return result.stdout.strip()


def _port(container, port):
    binding = _run(["docker", "port", container, f"{port}/tcp"])
    ip, sep, value = binding.partition(":")
    if ip != "127.0.0.1" or not sep or not value.isdecimal():
        raise RuntimeError(f"Unexpected Docker port binding for {container}: {binding}")
    return int(value)


@dataclass
class _Services:
    dsn: str
    redis_url: str
    _sql_helper: Path
    _redis_container: str

    def db(self, sql, args=None):
        return json.loads(_run(
            [str(self._sql_helper)], input=json.dumps({"sql": sql, "args": args or []}),
            env={**os.environ, "BILLING_EXACT_DSN": self.dsn}, timeout=15,
        ))

    def redis_cmd(self, *args):
        # RESP2 represents HGETALL as alternating fields and values; JSON
        # preserves empty strings, newlines and exact Redis integer replies.
        return json.loads(_run(["docker", "exec", self._redis_container,
                                "redis-cli", "--json", "-2", *map(str, args)]))

    def ping_redis_from_host(self):
        # The gateway connects to the published port, not the container's loopback.
        target = urlsplit(self.redis_url)
        with socket.create_connection((target.hostname, target.port), timeout=3) as sock:
            sock.sendall(b"PING\r\n")
            with sock.makefile("rb") as reply:
                if reply.readline(8) != b"+PONG\r\n":
                    raise RuntimeError("Published Redis port did not return PONG")


@contextmanager
def local_services(out: Path, *, check_running=lambda: None):
    """Start isolated services; remove only this run's containers and volumes."""
    out = Path(out).resolve()
    out.mkdir(parents=True, exist_ok=True)
    label = "billing-exact-run=" + secrets.token_hex(16)
    password = secrets.token_hex(20)
    with tempfile.TemporaryDirectory(prefix="services-", dir=out) as tmp:
        helper = Path(tmp) / "mysqlquery"
        _run(["go", "build", "-o", str(helper), "./scripts/billing_exact/mysqlquery"],
             cwd=ROOT, timeout=600)
        check_running()
        _run(["docker", "info", "--format", "{{.ServerVersion}}"])
        try:
            # Name/label scope is independent of external container names or
            # volumes. tmpfs overrides both images' data VOLUME declarations.
            check_running()
            mysql = _run([
                "docker", "run", "-d", "--label", label, "-p", "127.0.0.1::3306",
                "--tmpfs", "/var/lib/mysql", "-e", "MYSQL_ROOT_PASSWORD",
                "-e", "MYSQL_DATABASE=billing", "-e", "MYSQL_USER=billing",
                "-e", "MYSQL_PASSWORD", "mysql:5.7.44",
                "--character-set-server=utf8mb4", "--collation-server=utf8mb4_unicode_ci",
            ], env={**os.environ, "MYSQL_ROOT_PASSWORD": secrets.token_hex(20),
                    "MYSQL_PASSWORD": password}, timeout=300)
            check_running()
            redis = _run([
                "docker", "run", "-d", "--label", label, "-p", "127.0.0.1::6379",
                "--tmpfs", "/data", "redis:7-alpine", "redis-server",
                "--save", "", "--appendonly", "no",
            ], timeout=300)
            svc = _Services(
                f"billing:{password}@tcp(127.0.0.1:{_port(mysql, 3306)})/billing"
                "?parseTime=true&timeout=3s&readTimeout=12s&writeTimeout=12s",
                f"redis://127.0.0.1:{_port(redis, 6379)}/0", helper, redis,
            )
            deadline = time.monotonic() + 120
            while True:
                check_running()
                try:
                    svc.db("SELECT 1")
                    svc.ping_redis_from_host()
                    break
                except (OSError, RuntimeError) as exc:
                    if time.monotonic() >= deadline:
                        raise RuntimeError(f"Temporary databases did not become ready: {exc}") from exc
                    time.sleep(0.5)
            yield svc
        finally:
            containers = _run(["docker", "ps", "-aq", "--filter", f"label={label}"]).splitlines()
            if containers:
                _run(["docker", "rm", "--force", "--volumes", *containers], timeout=60)
