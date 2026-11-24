"""Run all exact billing cases against a fresh, local gateway and mock upstreams."""
import argparse
import concurrent.futures
import json
import os
import secrets
import signal
import socket
import subprocess
import sys
import tempfile
import threading
import time
from collections import Counter
from contextlib import ExitStack, contextmanager
from pathlib import Path

import requests

from local_services import local_services
from scenarios import CHAT_MODELS, IMAGE_MODELS, VIDEO_MODEL, ExactBillingCases
from upstream import Server

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / "scripts" / "seedance_mock"))
from server import MockServer as VideoServer


@contextmanager
def stop_on_sigterm():
    # Check between operations so a signal cannot orphan a starting process or interrupt cleanup.
    requested = False

    def request_stop(signum, frame):
        nonlocal requested
        requested = True

    def check_running():
        if requested:
            raise SystemExit(128 + signal.SIGTERM)

    previous = signal.signal(signal.SIGTERM, request_stop)
    try:
        yield check_running
        check_running()
    finally:
        signal.signal(signal.SIGTERM, previous)


@contextmanager
def serve(server):
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    try:
        yield server
    finally:
        server.shutdown()
        server.server_close()
        thread.join()


class Suite(ExactBillingCases):
    def __init__(self, binary, out, services, check_running):
        self.binary = binary
        self.OUT = out
        self.services = services
        self.check_running = check_running
        self.db = services.db
        self.redis_cmd = services.redis_cmd
        self.exact_results = []
        self.checks = []
        self.app = None
        self.starts = 0
        self.secrets = dict(SESSION_SECRET=secrets.token_hex(32), CRYPTO_SECRET=secrets.token_hex(32))
        self.models = [*CHAT_MODELS, *IMAGE_MODELS, "gpt-4o", "gpt-image-2.6-sunburst", VIDEO_MODEL]
        self.clients = threading.local()

    def check(self, name, passed, **detail):
        self.checks.append(dict(name=name, passed=bool(passed), **detail))
        if not passed:
            print("FAIL:", name, detail, flush=True)

    def client(self):
        self.check_running()
        if not hasattr(self.clients, "http"):
            self.clients.http = requests.Session()
            self.clients.http.trust_env = False
        return self.clients.http

    def request(self, path, method="GET", auth=None, body=None):
        headers = {"Authorization": "Bearer " + auth} if auth else {}
        return self.client().request(method, self.base + path, headers=headers, json=body,
                                     timeout=30, allow_redirects=False)

    def multipart_request(self, path, fields, files):
        return self.client().post(self.base + path, headers={"Authorization": "Bearer " + self.token},
                                  data=fields, files=files, timeout=30, allow_redirects=False)

    def call_check(self, name, path, body):
        response = self.request(path, "POST", self.token, body)
        self.check(name, response.status_code == 200, status=response.status_code)
        return response

    def api(self, path, method="GET", auth=None, body=None):
        response = self.request(path, method, auth, body)
        response.raise_for_status()
        obj = response.json()
        if not obj.get("success"):
            raise RuntimeError(f"{path}: {obj.get('message')}")
        return obj.get("data")

    def option(self, name, value):
        self.api("/api/option/", "PUT", self.admin, {
            "key": name, "value": value if isinstance(value, str) else json.dumps(value)})

    def start_app(self):
        self.check_running()
        if self.starts == 0:
            with socket.socket() as sock:
                sock.bind(("127.0.0.1", 0))
                self.port = sock.getsockname()[1]
        port = self.port
        self.base = f"http://127.0.0.1:{port}"
        runtime = self.OUT / "runtime"
        runtime.mkdir(exist_ok=True)
        env = {k: os.environ[k] for k in ("PATH", "HOME", "LANG", "TZ") if k in os.environ}
        env.update(self.secrets, SQL_DSN=self.services.dsn, REDIS_CONN_STRING=self.services.redis_url,
                   GIN_MODE="release", PORT=str(port), MEMORY_CACHE_ENABLED="true", SYNC_FREQUENCY="60",
                   SQL_MAX_OPEN_CONNS="100", SQL_MAX_IDLE_CONNS="100", REDIS_POOL_SIZE="10", GOMAXPROCS="4",
                   UPDATE_TASK="false", GENERATE_DEFAULT_TOKEN="false", GLOBAL_API_RATE_LIMIT_ENABLE="false",
                   GLOBAL_WEB_RATE_LIMIT_ENABLE="false", CRITICAL_RATE_LIMIT_ENABLE="false",
                   SHUTDOWN_TIMEOUT_SECONDS="5", HTTP_PROXY="http://127.0.0.1:9",
                   HTTPS_PROXY="http://127.0.0.1:9", NO_PROXY="127.0.0.1,localhost")
        self.starts += 1
        with (self.OUT / "private" / f"gateway-{self.starts}.log").open("wb") as log:
            self.app = subprocess.Popen([str(self.binary), "--listen-host", "127.0.0.1",
                                         "--log-dir", str(runtime / "logs")],
                                        cwd=runtime, env=env, stdout=log, stderr=log)
        deadline = time.monotonic() + 90
        while time.monotonic() < deadline and self.app.poll() is None:
            try:
                response = self.request("/api/ready")
                if response.ok and response.json()["data"]["checks"] == {
                    "database": "ok", "log_database": "ok", "redis": "ok"
                }:
                    return
            except requests.RequestException:
                pass
            time.sleep(.2)
        raise RuntimeError("gateway did not become ready; see private/gateway-*.log")

    def stop_app(self):
        app, self.app = self.app, None
        if app is None:
            return
        if app.poll() is None:
            app.send_signal(signal.SIGTERM)
        try:
            app.wait(timeout=15)
        except subprocess.TimeoutExpired:
            app.kill()
            app.wait()
        self.check(f"gateway-exit:{self.starts}", app.returncode == 0, code=app.returncode)

    def load(self, name, count, workers, fn):
        barrier = threading.Barrier(workers)

        def one(i):
            if i < workers:
                barrier.wait(timeout=20)
            response = fn(i)
            try:
                body = response.json()
            except requests.exceptions.JSONDecodeError:
                body = None
            return {"status": response.status_code, "body": body}

        with concurrent.futures.ThreadPoolExecutor(workers) as pool:
            rows = list(pool.map(one, range(count)))
        self.check("load:" + name, all(r["status"] == 200 for r in rows),
                   statuses=dict(Counter(r["status"] for r in rows)))
        return rows

    def setup(self, mock, video):
        self.upstream = mock
        password = secrets.token_urlsafe(12)
        self.api("/api/setup", "POST", body={"username": "mockadmin", "password": password,
                 "confirmPassword": password, "SelfUseModeEnabled": True, "DemoSiteEnabled": False})
        def login(user):
            return self.api("/api/user/login", "POST", body={
                "username": user, "password": password})["access_token"]
        self.admin = login("mockadmin")
        self.api("/api/user/", "POST", self.admin, {"username": "mockuser", "password": password, "role": 1})
        user = login("mockuser")
        self.user_id = self.api("/api/user/self", auth=user)["id"]
        self.api("/api/user/manage", "POST", self.admin,
                 {"id": self.user_id, "action": "add_quota", "mode": "add", "value": 50000000})
        tok = self.api("/api/token/", "POST", user, {"name": "mock", "expired_time": -1,
                       "remain_quota": 20000000, "unlimited_quota": False, "group": "default"})
        self.token_id = tok["id"]
        self.token = tok.get("key") or self.db("SELECT `key` FROM tokens WHERE id=?", [self.token_id])[0]["key"]
        for name, value in {"ModelRatio": dict.fromkeys(self.models, 1), "GroupRatio": {"default": 1},
                            "CompletionRatio": dict.fromkeys(self.models, 1), "RetryTimes": "0",
                            "ModelPrice": {VIDEO_MODEL: .001}, "LogConsumeEnabled": "true"}.items():
            self.option(name, value)
        def channel(name, kind, models, url, key="mock-local-key", **extra):
            c = dict(name=name, type=kind, models=",".join(models), group="default", key=key,
                     base_url=url, status=1, priority=0, weight=1, auto_ban=0, retry_enabled=False, **extra)
            self.api("/api/channel/", "POST", self.admin, {"mode": "single", "channel": c})
        channel("openai", 1, ["mock-chat", *IMAGE_MODELS], mock.url,
                model_mapping=json.dumps({"mock-chat": "gpt-4o", "image-alias": "gpt-image-2.6-sunburst"}),
                setting=json.dumps({"protocol": {"native_modes": [
                    "openai.chat", "openai.image.generations", "openai.image.edits"]}}))
        channel("claude", 14, ["claude-sonnet-4-5", "claude-opus-5-5"], mock.url)
        channel("gemini", 24, ["gemini-2.5-flash"], mock.url)
        channel("seedance", 45, [VIDEO_MODEL], video.public_url, key="seedance-mock-key")
        print("Waiting for new channels to enter the gateway cache ...", flush=True)
        deadline = time.monotonic() + 75
        while time.monotonic() < deadline:
            probe = self.request("/v1/chat/completions", "POST", self.token,
                                 {"model": "mock-chat", "messages": [{"role": "user", "content": "ready"}]})
            if probe.status_code != 503:
                probe.raise_for_status()
                return
            time.sleep(1)
        raise RuntimeError("new channels did not enter the routing cache")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", required=True, type=Path, help="gateway binary built from this checkout")
    args = parser.parse_args()
    binary = args.binary.resolve(strict=True)
    os.umask(0o077)
    out = Path(tempfile.mkdtemp(prefix="apimeter-exact-billing-"))
    (out / "private").mkdir()
    print("Results:", out, flush=True)
    with stop_on_sigterm() as check_running, local_services(out, check_running=check_running) as services, ExitStack() as stack:
        mock = stack.enter_context(serve(Server()))
        video = stack.enter_context(serve(VideoServer(("127.0.0.1", 0), scenario="success", step_seconds=0)))
        suite = Suite(binary, out, services, check_running)
        stack.callback(suite.stop_app)
        try:
            suite.start_app()
            suite.setup(mock, video)
            suite.test_exact_billing()
            suite.check("upstream:credentials", bool(mock.records) and all(r["auth_ok"] for r in mock.records))
        finally:
            suite.stop_app()
            (out / "checks.json").write_text(json.dumps(suite.checks, indent=2))
    passed = sum(r["passed"] for r in suite.exact_results)
    failed = sum(not c["passed"] for c in suite.checks)
    print(f"Exact accounting: {passed}/83; failed checks: {failed}")
    return int(passed != 83 or failed != 0)


if __name__ == "__main__":
    sys.exit(main())
