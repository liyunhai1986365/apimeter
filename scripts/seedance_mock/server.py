"""Local Ark video protocol fixture; no upstream requests or generation fees."""
import argparse
import copy
import json
import os
import threading
import time
import uuid
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from urllib.parse import urlsplit

TASKS = '/api/v3/contents/generations/tasks'
SCENARIOS = ('success', 'failed', 'expired', 'cancelled', 'create429', 'query429', 'query500', 'querytimeout', 'queryinvalid', 'querywrongid', 'intermediary', 'intermediarywrongid')


class MockServer(ThreadingHTTPServer):
    daemon_threads = True

    def __init__(self, address, *, token='seedance-mock-key', scenario='success',
                 step_seconds=2, timeout_seconds=30, public_url=None, video=None, frame=None):
        super().__init__(address, Handler)
        self.token = token
        self.scenario = scenario
        self.step_seconds = step_seconds
        self.timeout_seconds = timeout_seconds
        self.public_url = (public_url or f'http://127.0.0.1:{self.server_port}').rstrip('/')
        self.video = Path(video) if video else None
        self.frame = Path(frame) if frame else None
        self.tasks = {}
        self.records = []
        self.lock = threading.Lock()


class Handler(BaseHTTPRequestHandler):
    def log_message(self, *_):
        pass

    def reply(self, status, body, *, retry=False):
        data = json.dumps(body, ensure_ascii=False).encode()
        self.send_response(status)
        self.send_header('Content-Type', 'application/json')
        self.send_header('Content-Length', str(len(data)))
        self.send_header('X-Request-Id', str(uuid.uuid4()))
        if retry:
            self.send_header('Retry-After', '1')
        self.end_headers()
        try:
            self.wfile.write(data)
        except (BrokenPipeError, ConnectionResetError):
            pass

    def error(self, status, code, message):
        self.reply(status, {'error': {'code': code, 'message': message}}, retry=status == 429)

    def authenticated(self):
        if self.headers.get('Authorization') != f'Bearer {self.server.token}':
            self.error(401, 'AuthenticationError', 'Invalid mock API key')
            return False
        return True

    def record(self, body=None):
        # Record only protocol data, never Authorization or other credentials.
        with self.server.lock:
            self.server.records.append({'method': self.command, 'path': self.path,
                                        'body': copy.deepcopy(body)})

    def do_POST(self):
        if not self.authenticated():
            return
        if urlsplit(self.path).path != TASKS:
            return self.error(404, 'NotFound', 'Unknown mock endpoint')
        try:
            size = int(self.headers.get('Content-Length', '0'))
            if not 0 < size <= 1024 * 1024:
                return self.error(400, 'InvalidParameter', 'Expected JSON body up to 1 MiB')
            body = json.loads(self.rfile.read(size))
        except (ValueError, UnicodeDecodeError):
            return self.error(400, 'InvalidParameter', 'Invalid JSON body')
        self.record(body)
        if not isinstance(body, dict) or not isinstance(body.get('model'), str) or not body['model'] or not isinstance(body.get('content'), list) or not body['content']:
            return self.error(400, 'InvalidParameter', 'model and nonempty content are required')
        if self.server.scenario == 'create429':
            return self.error(429, 'TooManyRequests', 'Mock creation rate limit')
        official_id = 'cgt-' + uuid.uuid4().hex
        intermediary = self.server.scenario.startswith('intermediary')
        task_id = 'task_' + uuid.uuid4().hex if intermediary else official_id
        with self.server.lock:
            self.server.tasks[task_id] = {'request': copy.deepcopy(body), 'created_at': int(time.time()),
                                          'started': time.monotonic(), 'id': task_id, 'official_id': official_id, 'queries': 0}
        self.reply(200, {'id': task_id, 'upstream_task_id': official_id} if intermediary else {'id': task_id})

    def do_GET(self):
        path = urlsplit(self.path).path
        if path.startswith('/media/'):
            return self.media(path)
        if not self.authenticated():
            return
        if path == '/__mock/requests':
            with self.server.lock:
                records = copy.deepcopy(self.server.records)
            return self.reply(200, {'requests': records})
        if not path.startswith(TASKS + '/'):
            return self.error(404, 'NotFound', 'Unknown mock endpoint')
        self.record()
        task_id = path[len(TASKS) + 1:]
        with self.server.lock:
            stored = self.server.tasks.get(task_id)
            if stored is not None:
                stored['queries'] += 1
            task = copy.deepcopy(stored)
        if task is None:
            return self.error(404, 'TaskNotFound', 'Unknown mock task')
        scenario = self.server.scenario
        if scenario.startswith('intermediary'):
            if task['queries'] == 1:
                return self.reply(200, {'id': task_id, 'upstream_task_id': task['official_id']})
            body = self.task_response(task)
            body['upstream_task_id'] = 'cgt-wrong' if scenario == 'intermediarywrongid' else task['official_id']
            body['official_url'] = self.server.public_url + '/media/video'
            return self.reply(200, body)
        if scenario == 'query429':
            return self.error(429, 'TooManyRequests', 'Mock query rate limit')
        if scenario == 'query500':
            return self.error(500, 'InternalError', 'Mock upstream failure')
        if scenario == 'queryinvalid':
            return self.reply(200, {})
        if scenario == 'querywrongid':
            body = self.task_response(task)
            body['id'] = 'cgt-wrong-task'
            return self.reply(200, body)
        if scenario == 'querytimeout':
            time.sleep(self.server.timeout_seconds)
        self.reply(200, self.task_response(task))

    def task_response(self, task):
        req = task['request']
        age = time.monotonic() - task['started']
        step = self.server.step_seconds
        status = 'queued' if age < step else 'running' if age < step * 2 else 'succeeded'
        if status == 'succeeded' and self.server.scenario in ('failed', 'expired', 'cancelled'):
            status = self.server.scenario
        result = {'id': task['id'], 'model': req['model'], 'status': status,
                  'created_at': task['created_at'], 'updated_at': task['created_at'] + int(age), 'error': None}
        if status in ('failed', 'expired', 'cancelled'):
            result['error'] = {'code': {'expired': 'TaskExpired', 'failed': 'MockGenerationFailed',
                                       'cancelled': 'TaskCancelled'}[status], 'message': f'Mock task {status}'}
        if status != 'succeeded':
            return result
        # Deliberately includes extension fields to detect response whitelist loss.
        for key in ('seed', 'bitrate_mode', 'generate_audio', 'output_format', 'tools', 'safety_identifier', 'execution_expires_after'):
            if key in req:
                result[key] = copy.deepcopy(req[key])
        result['duration'] = 5 if req.get('duration', -1) == -1 else req['duration']
        result['content'] = {'video_url': self.server.public_url + '/media/video'}
        if req.get('return_last_frame') is True:
            result['content']['last_frame_url'] = self.server.public_url + '/media/frame'
        result['usage'] = {'completion_tokens': 100, 'total_tokens': 100,
                           'mock_token_details': {'zero': 0, 'nested': {'value': 7}}}
        if req.get('tools'):
            result['usage']['tool_usage'] = {'web_search': 0}
        result['mock_extension'] = {'preserve': True}
        return result

    def media(self, path):
        file = self.server.video if path == '/media/video' else self.server.frame if path == '/media/frame' else None
        if not file or not file.is_file():
            return self.error(404, 'MockFixtureMissing', 'Configure --video/--frame for downloadable media')
        data = file.read_bytes()
        self.send_response(200)
        self.send_header('Content-Type', 'video/quicktime' if file.suffix.lower() == '.mov' else 'video/mp4' if path == '/media/video' else 'image/png')
        self.send_header('Content-Length', str(len(data)))
        self.end_headers()
        try:
            self.wfile.write(data)
        except (BrokenPipeError, ConnectionResetError):
            pass


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--host', default='127.0.0.1')
    parser.add_argument('--port', type=int, default=18089)
    parser.add_argument('--scenario', choices=SCENARIOS, default='success')
    parser.add_argument('--step-seconds', type=float, default=2)
    parser.add_argument('--timeout-seconds', type=float, default=30)
    parser.add_argument('--public-url', help='Mock origin reachable by gateway and test client')
    parser.add_argument('--video', help='Existing MP4/MOV fixture; never generated by mock')
    parser.add_argument('--frame', help='Existing PNG fixture')
    args = parser.parse_args()
    if args.step_seconds < 0 or args.timeout_seconds < 0:
        parser.error('durations must be nonnegative')
    for file in (args.video, args.frame):
        if file and not Path(file).is_file():
            parser.error(f'fixture does not exist: {file}')
    server = MockServer((args.host, args.port), token=os.getenv('SEEDANCE_MOCK_KEY', 'seedance-mock-key'),
                        scenario=args.scenario, step_seconds=args.step_seconds,
                        timeout_seconds=args.timeout_seconds, public_url=args.public_url,
                        video=args.video, frame=args.frame)
    print(f'Seedance mock listening on {server.server_address}, scenario={args.scenario}', flush=True)
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        pass
    finally:
        server.server_close()


if __name__ == '__main__':
    main()
