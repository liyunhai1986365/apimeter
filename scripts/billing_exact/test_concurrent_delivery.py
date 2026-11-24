import unittest
from unittest.mock import Mock

from requests import Response

from run import Suite
from scenarios import ExactBillingCases
from upstream import PNG


def chat(text, *, role='assistant', finish='stop'):
    return {'object': 'chat.completion', 'choices': [
        {'message': {'role': role, 'content': text}, 'finish_reason': finish}]}


class ConcurrentDeliveryTest(unittest.TestCase):
    def test_each_chat_and_image_must_deliver_the_requested_result(self):
        text = {'mock-chat': 'mock openai answer', 'claude-sonnet-4-5': 'mock claude answer',
                'gemini-2.5-flash': 'mock gemini answer'}
        image = {'data': [{'b64_json': PNG}]}
        suite = ExactBillingCases()
        suite.check, suite.account_snapshot, suite.verify_charge = Mock(), Mock(), Mock()
        expected = {}

        def load(name, count, workers, request):
            if name.startswith('exact-video-'):
                return [{'status': 200, 'body': {'id': str(i), 'status': 'succeeded'}} for i in range(count)]
            model = name.removeprefix('exact-concurrent-')
            valid = chat(text[model]) if model in text else image
            invalid = [None, {}, [], {'error': {'message': 'upstream failed'}},
                       {**valid, 'error': {'message': 'partial result'}}]
            if model in text:
                invalid += [image, chat(''), chat('mock different answer'),
                            chat(next(t for t in text.values() if t != text[model])),
                            chat(text[model], finish='length'), chat(text[model], role='user'),
                            {'object': 'chat.completion', 'choices': []},
                            {**valid, 'object': 'chat.completion.chunk'}]
            else:
                invalid += [chat('mock openai answer'), {'data': []}, {'data': [{'b64_json': 'bad'}]},
                            {'data': valid['data'] * 2}]
            rows = [{'status': 200, 'body': valid} for _ in range(count)]
            for index, bad in enumerate(invalid, 1):
                rows[index] = {'status': 200, 'body': bad}
            rows[-1] = {'status': 502, 'body': valid}
            expected[model] = [*range(2, len(invalid) + 2), count]
            return rows

        suite.load = load
        suite.concurrent_cases(dict(name='baseline', mode='ratio', video_price='.001', group=1,
                                    model_ratio=1, completion=1, cache=1, creation=1))
        checked = {c.args[0].split(':')[1]: (c.args[1], c.kwargs['invalid_results'])
                   for c in suite.check.call_args_list if c.args[0].startswith('exact-concurrent-success:')}
        self.assertEqual(set(checked), {*text, 'gpt-image-2'})
        self.assertEqual(checked, {model: (False, indices) for model, indices in expected.items()})
        self.assertEqual(suite.verify_charge.call_count, 5)

    def test_empty_response_does_not_skip_the_rest_of_the_batch(self):
        suite = Suite.__new__(Suite)
        suite.check = Mock()
        def request(i):
            response = Response()
            response.status_code, response._content = 200, (b'' if i == 0 else b'{"data":[]}')
            return response
        self.assertEqual(suite.load('batch', 2, 2, request), [
            {'status': 200, 'body': None}, {'status': 200, 'body': {'data': []}}])
