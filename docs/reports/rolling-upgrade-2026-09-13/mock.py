import base64
import json
import time
import uuid
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

PNG = 'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aEtcAAAAASUVORK5CYII='

class Handler(BaseHTTPRequestHandler):
    protocol_version = 'HTTP/1.1'

    def log_message(self, *args):
        pass

    def reply(self, value, status=200):
        body = json.dumps(value).encode()
        self.send_response(status)
        self.send_header('Content-Type', 'application/json')
        self.send_header('Content-Length', str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_GET(self):
        self.reply({'object': 'list', 'data': [{'id': 'gpt-3.5-turbo', 'object': 'model', 'owned_by': 'test'}]})

    def do_POST(self):
        req = json.loads(self.rfile.read(int(self.headers.get('Content-Length', 0))))
        if '/images/generations' in self.path:
            time.sleep(10)
            return self.reply({'created': int(time.time()), 'data': [{'b64_json': PNG}]})
        content = str(req.get('messages', [{}])[-1].get('content', ''))
        if 'MOCK_FAIL' in content:
            return self.reply({'error': {'message': 'Controlled test failure', 'type': 'server_error', 'code': 'test_error'}}, 500)
        ident = 'chatcmpl-' + uuid.uuid4().hex
        common = {'id': ident, 'created': int(time.time()), 'model': req.get('model', 'gpt-3.5-turbo')}
        usage = {'prompt_tokens': 20, 'completion_tokens': 10, 'total_tokens': 30}
        if not req.get('stream'):
            time.sleep(0.05)
            return self.reply({**common, 'object': 'chat.completion', 'choices': [{'index': 0, 'message': {'role': 'assistant', 'content': 'compat-ok'}, 'finish_reason': 'stop'}], 'usage': usage})
        self.send_response(200)
        self.send_header('Content-Type', 'text/event-stream')
        self.send_header('Cache-Control', 'no-cache')
        self.send_header('Connection', 'close')
        self.end_headers()
        delay = 1 if 'MOCK_LONG' in content else 0.1
        count = 90 if 'MOCK_LONG' in content else 3
        try:
            for i in range(count):
                obj = {**common, 'object': 'chat.completion.chunk', 'choices': [{'index': 0, 'delta': {'content': 'compat-ok' if i == 0 else '.'}, 'finish_reason': None}]}
                self.wfile.write(('data: ' + json.dumps(obj) + '\n\n').encode())
                self.wfile.flush()
                time.sleep(delay)
            self.wfile.write(('data: ' + json.dumps({**common, 'object': 'chat.completion.chunk', 'choices': [{'index': 0, 'delta': {}, 'finish_reason': 'stop'}]}) + '\n\n').encode())
            self.wfile.write(('data: ' + json.dumps({**common, 'object': 'chat.completion.chunk', 'choices': [], 'usage': usage}) + '\n\ndata: [DONE]\n\n').encode())
            self.wfile.flush()
        except (BrokenPipeError, ConnectionResetError):
            pass
        self.close_connection = True

ThreadingHTTPServer(('0.0.0.0', 8080), Handler).serve_forever()
