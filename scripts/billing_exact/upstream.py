"""Loopback upstream for the exact billing HTTP suite (stdlib only)."""

import json
import threading
import time
from email.parser import BytesParser
from email.policy import default
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import parse_qs, urlsplit


PNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII="
API_KEY = "mock-local-key"
CHAT_USAGE = {"prompt_tokens": 100, "completion_tokens": 20, "total_tokens": 120}
IMAGE_USAGE = {"input_tokens": 100, "output_tokens": 20, "total_tokens": 120}


class Server(ThreadingHTTPServer):
    daemon_threads = True
    request_queue_size = 256

    def __init__(self):
        super().__init__(("127.0.0.1", 0), Handler)
        self.lock = threading.Lock()
        self.records = []

    @property
    def url(self):
        return f"http://127.0.0.1:{self.server_port}"


class Handler(BaseHTTPRequestHandler):
    def log_message(self, *args):
        pass

    def reply(self, body, status=200):
        data = json.dumps(body).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def sse(self, frames):
        self.send_response(200)
        self.send_header("Content-Type", "text/event-stream")
        self.send_header("Connection", "close")
        self.end_headers()
        for event, body in frames:
            data = body if isinstance(body, str) else json.dumps(body)
            prefix = f"event: {event}\n" if event else ""
            self.wfile.write(f"{prefix}data: {data}\n\n".encode())
            self.wfile.flush()

    def do_POST(self):
        url = urlsplit(self.path)
        auth_ok = (
            self.headers.get("Authorization") == "Bearer " + API_KEY
            or self.headers.get("x-api-key") == API_KEY
            or self.headers.get("x-goog-api-key") == API_KEY
            or parse_qs(url.query).get("key") == [API_KEY]
        )
        def record(model=None):
            with self.server.lock:
                self.server.records.append({"path": url.path, "auth_ok": auth_ok, "model": model})
        if not auth_ok:
            record()
            return self.reply({"error": {"message": "invalid mock credential"}}, 401)

        raw = self.rfile.read(int(self.headers.get("Content-Length", 0)))
        if url.path.endswith("/images/edits"):
            content_type = self.headers.get("Content-Type", "")
            message = BytesParser(policy=default).parsebytes(
                f"Content-Type: {content_type}\r\n\r\n".encode() + raw
            )
            record(next((part.get_content() for part in message.iter_parts()
                         if part.get_param('name', header='content-disposition') == 'model'), None))
            if not any(
                part.get_param("name", header="content-disposition") in ("image", "image[]")
                and part.get_filename() and part.get_payload(decode=True)
                for part in message.iter_parts()
            ):
                return self.reply({"error": {"message": "missing edit image"}}, 400)
            return self.images(1)
        try:
            body = json.loads(raw)
        except ValueError:
            record()
            return self.reply({"error": {"message": "invalid JSON"}}, 400)
        record(body.get('model'))
        if url.path.endswith("/images/generations"):
            if "MOCK_HOLD" in body.get("prompt", ""):
                time.sleep(0.8)
            return self.images(body.get("n", 1))
        if url.path.endswith("/chat/completions"):
            return self.chat(body)
        if url.path.endswith("/messages"):
            return self.claude(body)
        if url.path.endswith((":generateContent", ":streamGenerateContent")):
            return self.gemini(url.path.endswith(":streamGenerateContent"))
        self.reply({"error": {"message": "unknown mock route", "path": url.path}}, 404)

    def images(self, count):
        self.reply({"created": int(time.time()), "data": [{"b64_json": PNG}] * count, "usage": IMAGE_USAGE})

    def chat(self, body):
        common = {"id": "chatcmpl_mock", "created": int(time.time()), "model": body["model"]}
        message = {"role": "assistant", "content": "mock openai answer"}
        if not body.get("stream"):
            return self.reply({
                **common, "object": "chat.completion", "usage": CHAT_USAGE,
                "choices": [{"index": 0, "message": message, "finish_reason": "stop"}],
            })
        chunk = {**common, "object": "chat.completion.chunk"}
        start = {**chunk, "choices": [{"index": 0, "delta": message, "finish_reason": None}]}
        stop = {**chunk, "choices": [{"index": 0, "delta": {}, "finish_reason": "stop"}]}
        layout = body.get("messages", [{}])[0].get("content")
        if layout == "usage_before_stop":
            frames = [start, {**chunk, "choices": [], "usage": CHAT_USAGE}, stop]
        elif layout == "split_usage":
            frames = [{**start, "usage": {"prompt_tokens": CHAT_USAGE["prompt_tokens"]}},
                      {**stop, "usage": {"completion_tokens": CHAT_USAGE["completion_tokens"]}}]
        elif layout in ("update_prompt_and_total", "update_completion", "update_completion_and_total"):
            p, c = CHAT_USAGE["prompt_tokens"], CHAT_USAGE["completion_tokens"]
            early, final = {
                "update_prompt_and_total": ({"prompt_tokens": p, "completion_tokens": 4, "total_tokens": p + 4},
                                            {"prompt_tokens": p, "total_tokens": p + c}),
                "update_completion": ({"prompt_tokens": p, "total_tokens": p + 4}, {"completion_tokens": c}),
                "update_completion_and_total": ({"prompt_tokens": 2, "completion_tokens": c, "total_tokens": 2 + c},
                                                {"completion_tokens": c, "total_tokens": p + c}),
            }[layout]
            frames = [{**start, "usage": early}, {**stop, "usage": final}]
        else:
            frames = [start, {**stop, "usage": CHAT_USAGE}]
        self.sse([(None, frame) for frame in [*frames, "[DONE]"]])

    def claude(self, body):
        usage = {"input_tokens": 100, "output_tokens": 20, "cache_creation_input_tokens": 10, "cache_read_input_tokens": 5,
                 "cache_creation": {"ephemeral_5m_input_tokens": 3, "ephemeral_1h_input_tokens": 7}}
        text = "mock claude answer"
        message = {
            "id": "msg_mock", "type": "message", "role": "assistant", "model": body["model"],
            "content": [{"type": "text", "text": text}], "stop_reason": "end_turn", "stop_sequence": None, "usage": usage,
        }
        if not body.get("stream"):
            return self.reply(message)
        frames = [
            ("message_start", {"message": {**message, "content": [], "stop_reason": None, "usage": {**usage, "output_tokens": 0}}}),
            ("content_block_start", {"index": 0, "content_block": {"type": "text", "text": ""}}),
            ("content_block_delta", {"index": 0, "delta": {"type": "text_delta", "text": text}}),
            ("content_block_stop", {"index": 0}),
            ("message_delta", {"delta": {"stop_reason": "end_turn", "stop_sequence": None}, "usage": {"output_tokens": 20}}),
            ("message_stop", {}),
        ]
        self.sse((event, {"type": event, **fields}) for event, fields in frames)

    def gemini(self, stream):
        result = {
            "candidates": [{"index": 0, "content": {"role": "model", "parts": [{"text": "mock gemini answer"}]}, "finishReason": "STOP"}],
            "usageMetadata": {"promptTokenCount": 100, "candidatesTokenCount": 20, "totalTokenCount": 120},
            "modelVersion": "gemini-2.5-flash",
        }
        if stream:
            return self.sse([(None, result)])
        self.reply(result)


if __name__ == "__main__":
    with Server() as server:
        print(server.url, flush=True)
        server.serve_forever()
