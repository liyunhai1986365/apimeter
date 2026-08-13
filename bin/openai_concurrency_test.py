#!/usr/bin/env python3
"""Concurrent load test for an OpenAI-compatible Chat Completions endpoint."""

from __future__ import annotations

import argparse
import json
import math
import os
import re
import statistics
import sys
import threading
import time
import urllib.error
import urllib.parse
import urllib.request
from collections import Counter
from concurrent.futures import ThreadPoolExecutor, as_completed
from dataclasses import dataclass
from pathlib import Path
from typing import Iterable, List, Optional, Sequence, Tuple


DEFAULT_TOKEN_PROFILES = "small:128,large:2048"
DEFAULT_REQUESTS = 100
DEFAULT_CONCURRENCY = 100
DEFAULT_TIMEOUT_SECONDS = 180.0
DEFAULT_MODEL = "gpt-3.5-turbo"
DOTENV_KEY_PATTERN = re.compile(r"^[A-Za-z_][A-Za-z0-9_]*$")


@dataclass(frozen=True)
class RequestResult:
    status: int
    latency_seconds: float
    prompt_tokens: Optional[int] = None
    completion_tokens: Optional[int] = None
    total_tokens: Optional[int] = None
    error: Optional[str] = None

    @property
    def succeeded(self) -> bool:
        return 200 <= self.status < 300 and self.error is None


def positive_int(value: str) -> int:
    parsed = int(value)
    if parsed <= 0:
        raise argparse.ArgumentTypeError("must be greater than 0")
    return parsed


def load_dotenv(path: Path) -> dict:
    """Read dotenv values as data without sourcing or expanding shell code."""
    if not path.exists():
        return {}
    if not path.is_file():
        raise ValueError(f"config path is not a file: {path}")

    values = {}
    try:
        lines = path.read_text(encoding="utf-8").splitlines()
    except OSError as exc:
        raise ValueError(f"cannot read config file: {path}") from exc

    for line in lines:
        stripped = line.strip()
        if not stripped or stripped.startswith("#"):
            continue
        if stripped.startswith("export "):
            stripped = stripped[len("export ") :].lstrip()
        if "=" not in stripped:
            continue

        key, raw_value = stripped.split("=", 1)
        key = key.strip()
        if not DOTENV_KEY_PATTERN.fullmatch(key):
            continue

        value = raw_value.strip()
        if len(value) >= 2 and value[0] == value[-1] and value[0] in ("'", '"'):
            value = value[1:-1]
        else:
            value = re.split(r"\s+#", value, maxsplit=1)[0].rstrip()
        values[key] = value

    return values


def configured_value(
    cli_value: object,
    names: Sequence[str],
    dotenv: dict,
    default: object = None,
) -> object:
    if cli_value is not None:
        return cli_value
    for name in names:
        environment_value = os.environ.get(name)
        if environment_value:
            return environment_value
    for name in names:
        dotenv_value = dotenv.get(name)
        if dotenv_value:
            return dotenv_value
    return default


def parse_token_profiles(value: str) -> List[Tuple[str, int]]:
    profiles: List[Tuple[str, int]] = []
    seen_labels = set()

    for raw_profile in value.split(","):
        raw_profile = raw_profile.strip()
        if not raw_profile:
            continue
        if ":" not in raw_profile:
            raise argparse.ArgumentTypeError(
                "token profiles must use label:max_tokens, for example small:128,large:2048"
            )

        label, raw_tokens = (part.strip() for part in raw_profile.split(":", 1))
        if not label:
            raise argparse.ArgumentTypeError("token profile label cannot be empty")
        if label in seen_labels:
            raise argparse.ArgumentTypeError(f"duplicate token profile label: {label}")
        try:
            max_tokens = positive_int(raw_tokens)
        except (ValueError, argparse.ArgumentTypeError) as exc:
            raise argparse.ArgumentTypeError(
                f"invalid max_tokens for profile {label}: {raw_tokens}"
            ) from exc

        profiles.append((label, max_tokens))
        seen_labels.add(label)

    if not profiles:
        raise argparse.ArgumentTypeError("at least one token profile is required")
    return profiles


def chat_completions_url(endpoint: str) -> str:
    endpoint = endpoint.strip()
    if not endpoint:
        raise ValueError("endpoint cannot be empty")
    if "://" not in endpoint:
        endpoint = f"https://{endpoint}"

    parsed = urllib.parse.urlsplit(endpoint)
    if parsed.scheme not in ("http", "https") or not parsed.netloc:
        raise ValueError("endpoint must be a valid HTTP(S) URL or domain")

    path = parsed.path.rstrip("/")
    if path.endswith("/v1/chat/completions"):
        final_path = path
    elif path.endswith("/v1"):
        final_path = f"{path}/chat/completions"
    else:
        final_path = f"{path}/v1/chat/completions"

    return urllib.parse.urlunsplit(
        (parsed.scheme, parsed.netloc, final_path, parsed.query, "")
    )


def percentile(values: Sequence[float], percent: float) -> float:
    if not values:
        return 0.0
    ordered = sorted(values)
    rank = max(1, math.ceil(percent / 100 * len(ordered)))
    return ordered[rank - 1]


def make_payload(model: str, max_tokens: int) -> bytes:
    payload = {
        "model": model,
        "messages": [
            {
                "role": "user",
                "content": (
                    "This is a load test. Produce a coherent plain-text response and "
                    f"continue until you are close to the {max_tokens}-token output limit. "
                    "Do not use Markdown code fences."
                ),
            }
        ],
        "stream": False,
        "max_tokens": max_tokens,
    }
    return json.dumps(payload, ensure_ascii=False, separators=(",", ":")).encode(
        "utf-8"
    )


def integer_usage(usage: object, key: str) -> Optional[int]:
    if not isinstance(usage, dict):
        return None
    value = usage.get(key)
    if isinstance(value, bool):
        return None
    if isinstance(value, (int, float)):
        return int(value)
    return None


def execute_request(
    *,
    url: str,
    api_key: str,
    payload: bytes,
    timeout_seconds: float,
    start_event: threading.Event,
) -> RequestResult:
    start_event.wait()
    request = urllib.request.Request(
        url,
        data=payload,
        method="POST",
        headers={
            "Authorization": f"Bearer {api_key}",
            "Content-Type": "application/json",
            "Accept": "application/json",
            "User-Agent": "new-api-openai-concurrency-test/1.0",
        },
    )
    started_at = time.perf_counter()

    try:
        with urllib.request.urlopen(request, timeout=timeout_seconds) as response:
            status = response.getcode()
            response_body = response.read()
        elapsed = time.perf_counter() - started_at

        try:
            decoded = json.loads(response_body)
        except (json.JSONDecodeError, UnicodeDecodeError):
            return RequestResult(
                status=status,
                latency_seconds=elapsed,
                error="invalid JSON response",
            )

        usage = decoded.get("usage") if isinstance(decoded, dict) else None
        return RequestResult(
            status=status,
            latency_seconds=elapsed,
            prompt_tokens=integer_usage(usage, "prompt_tokens"),
            completion_tokens=integer_usage(usage, "completion_tokens"),
            total_tokens=integer_usage(usage, "total_tokens"),
        )
    except urllib.error.HTTPError as exc:
        # Drain the response so the connection can close cleanly, but never print
        # response bodies because they may contain provider or customer data.
        exc.read()
        return RequestResult(
            status=exc.code,
            latency_seconds=time.perf_counter() - started_at,
            error=f"HTTP {exc.code}",
        )
    except urllib.error.URLError as exc:
        reason = exc.reason
        error_name = reason.__class__.__name__ if reason is not None else "URLError"
        return RequestResult(
            status=0,
            latency_seconds=time.perf_counter() - started_at,
            error=error_name,
        )
    except (TimeoutError, OSError) as exc:
        return RequestResult(
            status=0,
            latency_seconds=time.perf_counter() - started_at,
            error=exc.__class__.__name__,
        )


def average_present(results: Iterable[RequestResult], field: str) -> Optional[float]:
    values = [getattr(result, field) for result in results]
    present = [value for value in values if value is not None]
    if not present:
        return None
    return statistics.fmean(present)


def format_optional(value: Optional[float]) -> str:
    return "n/a" if value is None else f"{value:.1f}"


def print_summary(
    *,
    label: str,
    max_tokens: int,
    results: Sequence[RequestResult],
    wall_seconds: float,
) -> bool:
    successful = [result for result in results if result.succeeded]
    failed = len(results) - len(successful)
    latencies = [result.latency_seconds for result in results]
    status_counts = Counter(str(result.status or "network") for result in results)
    error_counts = Counter(
        result.error for result in results if result.error is not None
    )

    average_completion = average_present(successful, "completion_tokens")
    saturation = (
        None
        if average_completion is None
        else average_completion / max_tokens * 100
    )

    print(f"\n[{label}] max_tokens={max_tokens}")
    print(
        f"  requests={len(results)} success={len(successful)} failed={failed} "
        f"success_rate={len(successful) / len(results) * 100:.2f}%"
    )
    print(
        f"  wall={wall_seconds:.3f}s throughput={len(results) / wall_seconds:.2f} req/s"
    )
    print(
        "  latency="
        f"avg {statistics.fmean(latencies):.3f}s, "
        f"p50 {percentile(latencies, 50):.3f}s, "
        f"p95 {percentile(latencies, 95):.3f}s, "
        f"p99 {percentile(latencies, 99):.3f}s, "
        f"max {max(latencies):.3f}s"
    )
    print(
        "  usage(avg successful)="
        f"prompt {format_optional(average_present(successful, 'prompt_tokens'))}, "
        f"completion {format_optional(average_completion)}, "
        f"total {format_optional(average_present(successful, 'total_tokens'))}, "
        f"output_limit_used {format_optional(saturation)}%"
    )
    print(f"  status={dict(sorted(status_counts.items()))}")
    if error_counts:
        print(f"  errors={dict(error_counts.most_common())}")

    return failed == 0


def run_profile(
    *,
    label: str,
    max_tokens: int,
    requests: int,
    concurrency: int,
    url: str,
    api_key: str,
    model: str,
    timeout_seconds: float,
) -> bool:
    payload = make_payload(model, max_tokens)
    start_event = threading.Event()
    workers = min(concurrency, requests)
    print(
        f"\nStarting profile={label} max_tokens={max_tokens} "
        f"requests={requests} concurrency={workers}"
    )

    started_at = time.perf_counter()
    with ThreadPoolExecutor(max_workers=workers) as executor:
        futures = [
            executor.submit(
                execute_request,
                url=url,
                api_key=api_key,
                payload=payload,
                timeout_seconds=timeout_seconds,
                start_event=start_event,
            )
            for _ in range(requests)
        ]
        start_event.set()
        results = [future.result() for future in as_completed(futures)]
    wall_seconds = time.perf_counter() - started_at

    return print_summary(
        label=label,
        max_tokens=max_tokens,
        results=results,
        wall_seconds=wall_seconds,
    )


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        description=(
            "Run separate concurrent load-test rounds for small and large "
            "max_tokens values against an OpenAI-compatible endpoint."
        ),
    )
    parser.add_argument(
        "endpoint",
        nargs="?",
        help=(
            "domain, API base URL, or full /v1/chat/completions URL "
            "(defaults to OPENAI_BASE_URL from the config file)"
        ),
    )
    parser.add_argument(
        "--config",
        default=os.environ.get("OPENAI_TEST_CONFIG", ".env"),
        help="local dotenv config file (default: .env)",
    )
    parser.add_argument(
        "--model",
        default=None,
        help=f"model name (config: OPENAI_MODEL; fallback: {DEFAULT_MODEL})",
    )
    parser.add_argument(
        "--api-key",
        default=None,
        help="API key; defaults to OPENAI_API_KEY from the config file",
    )
    parser.add_argument(
        "--requests",
        type=positive_int,
        default=None,
        help=f"request count per profile (config: OPENAI_TEST_REQUESTS; fallback: {DEFAULT_REQUESTS})",
    )
    parser.add_argument(
        "--concurrency",
        type=positive_int,
        default=None,
        help=(
            "maximum concurrent requests per profile "
            f"(config: OPENAI_TEST_CONCURRENCY; fallback: {DEFAULT_CONCURRENCY})"
        ),
    )
    parser.add_argument(
        "--token-profiles",
        type=parse_token_profiles,
        default=None,
        metavar="LABEL:TOKENS,...",
        help=(
            "comma-separated output max_tokens profiles "
            f"(config: OPENAI_TEST_TOKEN_PROFILES; fallback: {DEFAULT_TOKEN_PROFILES})"
        ),
    )
    parser.add_argument(
        "--timeout",
        type=float,
        default=None,
        help=(
            "per-request timeout in seconds "
            f"(config: OPENAI_TEST_TIMEOUT; fallback: {DEFAULT_TIMEOUT_SECONDS:g})"
        ),
    )
    return parser


def main(argv: Optional[Sequence[str]] = None) -> int:
    parser = build_parser()
    args = parser.parse_args(argv)

    config_path = Path(args.config).expanduser()
    try:
        dotenv = load_dotenv(config_path)
    except ValueError as exc:
        parser.error(str(exc))

    endpoint = configured_value(
        args.endpoint,
        ("OPENAI_BASE_URL", "OPENAI_API_BASE", "MODELSELL_BASE_URL", "FRONTEND_BASE_URL"),
        dotenv,
    )
    api_key = configured_value(
        args.api_key,
        ("OPENAI_API_KEY", "MODELSELL_API_KEY"),
        dotenv,
    )
    model = configured_value(
        args.model,
        ("OPENAI_MODEL", "MODELSELL_MODEL"),
        dotenv,
        DEFAULT_MODEL,
    )
    requests_value = configured_value(
        args.requests,
        ("OPENAI_TEST_REQUESTS",),
        dotenv,
        DEFAULT_REQUESTS,
    )
    concurrency_value = configured_value(
        args.concurrency,
        ("OPENAI_TEST_CONCURRENCY",),
        dotenv,
        DEFAULT_CONCURRENCY,
    )
    token_profiles_value = configured_value(
        args.token_profiles,
        ("OPENAI_TEST_TOKEN_PROFILES",),
        dotenv,
        DEFAULT_TOKEN_PROFILES,
    )
    timeout_value = configured_value(
        args.timeout,
        ("OPENAI_TEST_TIMEOUT",),
        dotenv,
        DEFAULT_TIMEOUT_SECONDS,
    )

    try:
        requests = (
            requests_value
            if isinstance(requests_value, int)
            else positive_int(str(requests_value))
        )
        concurrency = (
            concurrency_value
            if isinstance(concurrency_value, int)
            else positive_int(str(concurrency_value))
        )
        token_profiles = (
            token_profiles_value
            if isinstance(token_profiles_value, list)
            else parse_token_profiles(str(token_profiles_value))
        )
        timeout_seconds = float(timeout_value)
    except (ValueError, argparse.ArgumentTypeError) as exc:
        parser.error(f"invalid load-test setting: {exc}")

    if timeout_seconds <= 0:
        parser.error("--timeout must be greater than 0")
    if not endpoint:
        parser.error(
            f"set OPENAI_BASE_URL in {config_path} or pass an endpoint"
        )
    if not api_key:
        parser.error(
            f"set OPENAI_API_KEY in {config_path} or pass --api-key"
        )

    try:
        url = chat_completions_url(str(endpoint))
    except ValueError as exc:
        parser.error(str(exc))

    print("OpenAI-compatible concurrency test")
    print(f"  config={config_path} loaded={config_path.is_file()}")
    print(f"  endpoint={url}")
    print(f"  model={model}")
    print(
        f"  profiles={','.join(f'{label}:{tokens}' for label, tokens in token_profiles)}"
    )
    print(
        f"  requests/profile={requests} concurrency={concurrency} "
        f"total_requests={requests * len(token_profiles)}"
    )
    print("  note=max_tokens is a ceiling; actual completion usage is reported below")

    all_succeeded = True
    for label, max_tokens in token_profiles:
        profile_succeeded = run_profile(
            label=label,
            max_tokens=max_tokens,
            requests=requests,
            concurrency=concurrency,
            url=url,
            api_key=str(api_key),
            model=str(model),
            timeout_seconds=timeout_seconds,
        )
        all_succeeded = all_succeeded and profile_succeeded

    return 0 if all_succeeded else 1


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except KeyboardInterrupt:
        print("\nInterrupted", file=sys.stderr)
        raise SystemExit(130)
