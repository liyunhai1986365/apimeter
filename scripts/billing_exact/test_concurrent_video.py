import unittest
import threading
from unittest.mock import Mock

from scenarios import ExactBillingCases
from upstream import PNG


class ConcurrentVideoTest(unittest.TestCase):
    def test_each_repeated_image_and_video_query_matches_the_completed_task(self):
        profile = dict(name="baseline", mode="ratio", group="1", video_price=".001",
                       model_ratio="1", completion="1", cache="1", creation="1")
        for kind, field in [('image', 'state'), ('video', 'status')]:
            with self.subTest(kind=kind):
                suite = ExactBillingCases()
                suite.account_snapshot = Mock()
                suite.verify_charge = Mock()
                suite.token = "token"
                result = Mock(status_code=200)
                result.json.return_value = {'id': 'original', field: 'succeeded', 'data': {'images': [{'b64_json': PNG}]}}
                suite.call_check = suite.request = Mock(return_value=result)
                suite.db = Mock(return_value=[{'quota': 0}])
                rows = [{'status': 200, 'body': result.json()} for _ in range(20)]
                rows[3] = {'status': 200, 'body': {'id': 'other', field: 'succeeded'}}
                rows[-1] = {'status': 200, 'body': {'id': 'original', field: 'queued'}}
                suite.load = Mock(return_value=rows)
                suite.check = Mock()
                if kind == 'image':
                    suite.image_case(profile, 'gpt-image-2', asynchronous=True)
                else:
                    suite.video_case(profile)
                checks = [c.args[1] for c in suite.check.call_args_list
                          if c.args[0].startswith('exact-task-repeat:')]
                self.assertEqual(checks, [i not in (3, 19) for i in range(20)])
                self.assertEqual(suite.verify_charge.call_args.args[2], 0)
                self.assertEqual(suite.verify_charge.call_args.kwargs['log_count'], 0)

    def test_rejects_pending_or_mismatched_results_with_http_200(self):
        ids = [f"task-{i}" for i in range(10)]

        def load(name, count, workers, fn):
            if name == "exact-video-concurrent-create":
                return [{"status": 200, "body": {"id": i}} for i in ids]
            if name == "exact-video-concurrent-fetch":
                rows = [{"status": 200, "body": {"id": i, "status": "succeeded"}} for i in ids]
                rows[0]["body"]["status"] = "queued"
                rows[1]["body"]["id"] = ids[2]
                return rows
            return [{"status": 200}] * count

        suite = ExactBillingCases()
        suite.account_snapshot = Mock()
        suite.verify_charge = Mock()
        suite.load = load
        suite.check = Mock()
        suite.concurrent_cases(dict(name="baseline", group=1, mode="ratio", video_price=".001",
                                    model_ratio=1, completion=1, cache=1, creation=1))
        checks = [call for call in suite.check.call_args_list
                  if call.args[0].startswith("exact-task-terminal:concurrent/video/")]
        self.assertEqual([call.kwargs["expected_id"] for call in checks], ids)
        self.assertEqual([call.args[1] for call in checks], [False, False] + [True] * 8)

    def test_restart_requires_original_id_and_success_for_both_task_formats(self):
        suite = ExactBillingCases()
        suite.token = "token"
        suite.account_snapshot = Mock()
        suite.stop_app = Mock()
        suite.start_app = Mock()
        suite.verify_charge = Mock()
        suite.check = Mock()
        completed, responses, expected = [], [], []
        for kind, field in [('image', 'state'), ('video', 'status')]:
            for status, returned, state, ok in [(200, 'id', 'succeeded', True),
                                               (200, 'other', 'succeeded', False),
                                               (200, 'id', 'queued', False),
                                               (200, 'id', 'failed', False),
                                               (404, 'id', 'succeeded', False)]:
                completed.append((kind, '/tasks/id', 1 if kind == 'image' else None))
                response = Mock(status_code=status)
                response.json.return_value = {'id': returned, field: state, 'data': {'images': [{'b64_json': PNG}]}}
                responses.append(response)
                expected.append(ok)
        suite.request = Mock(side_effect=responses)
        suite.check_tasks_after_restart(completed)
        self.assertEqual([call.args[1] for call in suite.check.call_args_list], expected)
        suite.verify_charge.assert_called_once()

    def test_wrong_wire_model_cannot_pass_with_correct_billing(self):
        suite = ExactBillingCases()
        suite.check = Mock()
        suite.upstream = Mock(lock=threading.Lock())
        path, target = '/v1/images/generations', 'gpt-image-2.6-sunburst'
        suite.upstream.records = [{'path': path, 'model': target}]
        for observed, ok in [([], False), (['image-alias'], False), ([target, 'image-alias'], False), ([target], True)]:
            suite.upstream.records[1:] = [dict(path=path, model=model) for model in observed]
            suite.check_upstream_model('case', 1, '/images/generations', target)
            self.assertEqual(suite.check.call_args.args[1], ok)

    def test_succeeded_image_result_requires_expected_count_and_usable_content(self):
        import base64
        suite = ExactBillingCases()
        suite.base = 'http://127.0.0.1:8080'
        suite.check = Mock()
        suite.request = Mock(return_value=Mock(status_code=200, content=base64.b64decode(PNG)))
        inline, url = {'b64_json': PNG}, {'url': '/api/relay-temp-images/id.png'}
        for images, ok in [([inline, url], True), ([inline], False), ([inline, {}], False),
                           ([inline, {'url': 'https://elsewhere.invalid/image.png'}], False)]:
            suite.check_task_result('case', 'image', 'id', 200,
                                    dict(id='id', state='succeeded', data={'images': images}), 2)
            self.assertEqual(suite.check.call_args.args[1], ok)
        for status, content in [(404, base64.b64decode(PNG)), (200, b'not an image')]:
            suite.request.return_value = Mock(status_code=status, content=content)
            self.assertFalse(suite.images_match([url], 1))
