import json
import threading
import unittest
from urllib.error import HTTPError
from urllib.request import Request, urlopen

from server import MockServer, TASKS


class MockContractTest(unittest.TestCase):
    def setUp(self):
        self.server = MockServer(('127.0.0.1', 0), step_seconds=0)
        self.thread = threading.Thread(target=self.server.serve_forever, daemon=True)
        self.thread.start()
        self.origin = f'http://127.0.0.1:{self.server.server_port}'

    def tearDown(self):
        self.server.shutdown()
        self.server.server_close()
        self.thread.join()

    def call(self, path, body=None, token='seedance-mock-key'):
        request = Request(self.origin + path,
                          data=json.dumps(body).encode() if body is not None else None,
                          headers={'Authorization': 'Bearer ' + token, 'Content-Type': 'application/json'})
        try:
            response = urlopen(request, timeout=2)
        except HTTPError as error:
            response = error
        with response:
            return response.status, json.load(response), response.headers

    def create(self):
        req = {'model': 'mock-seedance', 'content': [{'type': 'text', 'text': 'test'}],
               'seed': 0, 'duration': -1, 'return_last_frame': True, 'generate_audio': False,
               'tools': [{'type': 'web_search', 'options': {'enabled': False}}],
               'bitrate_mode': 'test-only', 'output_format': 'mov'}
        status, body, _ = self.call(TASKS, req)
        self.assertEqual(status, 200)
        return body['id'], req

    def test_create_query_capture_and_extensions(self):
        task_id, req = self.create()
        self.assertTrue(task_id.startswith('cgt-'))
        status, body, headers = self.call(TASKS + '/' + task_id)
        self.assertEqual(status, 200)
        self.assertEqual(body['id'], task_id)
        self.assertEqual(body['status'], 'succeeded')
        self.assertEqual(body['duration'], 5)
        self.assertEqual(body['seed'], 0)
        self.assertIs(body['generate_audio'], False)
        self.assertEqual(body['tools'], req['tools'])
        self.assertEqual(body['usage']['tool_usage']['web_search'], 0)
        self.assertEqual(body['usage']['mock_token_details']['zero'], 0)
        self.assertTrue(body['content']['last_frame_url'].endswith('/media/frame'))
        self.assertTrue(headers['X-Request-Id'])
        _, captured, _ = self.call('/__mock/requests')
        self.assertEqual(captured['requests'][0]['body'], req)
        self.assertNotIn('Authorization', json.dumps(captured))
        self.assertEqual(self.call('/media/video')[0], 404)

    def test_timed_states_and_false_last_frame(self):
        task_id, _ = self.create()
        self.server.step_seconds = 10
        self.assertEqual(self.call(TASKS + '/' + task_id)[1]['status'], 'queued')
        with self.server.lock:
            self.server.tasks[task_id]['started'] -= 11
        self.assertEqual(self.call(TASKS + '/' + task_id)[1]['status'], 'running')
        with self.server.lock:
            self.server.tasks[task_id]['started'] -= 11
            self.server.tasks[task_id]['request']['return_last_frame'] = False
        result = self.call(TASKS + '/' + task_id)[1]
        self.assertEqual(result['status'], 'succeeded')
        self.assertNotIn('last_frame_url', result['content'])

    def test_auth_and_unknown_ids(self):
        self.assertEqual(self.call('/__mock/requests', token='wrong')[0], 401)
        self.assertEqual(self.call(TASKS + '/missing')[0], 404)
        self.assertEqual(self.call(TASKS, ['invalid'])[0], 400)

    def test_terminal_and_http_errors(self):
        task_id, req = self.create()
        for scenario in ('failed', 'expired', 'cancelled'):
            self.server.scenario = scenario
            status, body, _ = self.call(TASKS + '/' + task_id)
            self.assertEqual(status, 200)
            self.assertEqual(body['status'], scenario)
            self.assertTrue(body['error']['code'])
            self.assertNotIn('content', body)
            self.assertNotIn('usage', body)
        for scenario, expected in [('query429', 429), ('query500', 500)]:
            self.server.scenario = scenario
            status, body, headers = self.call(TASKS + '/' + task_id)
            self.assertEqual(status, expected)
            if expected == 429:
                self.assertEqual(headers['Retry-After'], '1')
        self.server.scenario = 'create429'
        self.assertEqual(self.call(TASKS, req)[0], 429)
        self.assertEqual(len(self.server.tasks), 1)


if __name__ == '__main__':
    unittest.main()
