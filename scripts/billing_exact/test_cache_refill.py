import tempfile
import unittest
from pathlib import Path
from unittest.mock import Mock, call, patch

from requests.exceptions import Timeout

from scenarios import ExactBillingCases


class CacheRefillTest(unittest.TestCase):
    def setUp(self):
        suite = self.suite = ExactBillingCases()
        directory = tempfile.TemporaryDirectory()
        self.addCleanup(directory.cleanup)
        suite.OUT = Path(directory.name)
        (suite.OUT / 'private').mkdir()
        suite.user_id, suite.token = 7, 'local-token'
        suite.check, suite.redis_cmd, suite.request = Mock(), Mock(), Mock()
        suite.exact_results = []
        self.before = dict(user_quota=10, user_used=0, token_remaining=10, token_used=0)
        suite.account_snapshot = Mock(return_value=dict(user_quota=0, user_used=10, token_remaining=0, token_used=10))
        suite.ledger_after = Mock(return_value=[dict(type=2, quota=10, model_name='mock')])

    def verify(self, *redis_replies, clock=(0, 7)):
        self.suite.redis_cmd.side_effect = redis_replies
        with patch('scenarios.time.monotonic', side_effect=clock):
            self.suite.verify_charge('cache', self.before, 10, '10', model='mock', log_count=1, per_log=10)
        return self.suite.exact_results[-1]['passed']

    def test_existing_hash_is_checked_without_refill(self):
        for fields, expected in [(['Quota', '0'], True), (['Quota', '1'], False), (['Id', '7'], False)]:
            with self.subTest(fields=fields):
                self.assertEqual(self.verify(fields), expected)
                self.suite.request.assert_not_called()

    def test_absent_key_requires_matching_quota_after_one_authenticated_read(self):
        self.suite.request.return_value.status_code = 200
        for quota in ['0', '1', None]:
            with self.subTest(quota=quota):
                self.suite.request.reset_mock()
                self.suite.redis_cmd.reset_mock()
                self.assertEqual(self.verify([], quota), quota == '0')
                self.suite.request.assert_called_once_with('/v1/models', auth='local-token')
                self.assertEqual(self.suite.redis_cmd.call_args_list,
                                 [call('HGETALL', 'user:7'), call('HGET', 'user:7', 'Quota')])

    def test_failed_read_cannot_pass_even_if_another_request_refilled_the_key(self):
        self.suite.request.return_value.status_code = 401
        self.assertFalse(self.verify([], '0'))
        self.suite.request.side_effect = Timeout()
        self.assertFalse(self.verify([], '0'))

    def test_unsettled_ledger_cannot_trigger_refill(self):
        self.suite.ledger_after.return_value = []
        self.assertFalse(self.verify([]))
        self.suite.request.assert_not_called()

    def test_wrong_quota_expiring_during_polling_cannot_trigger_refill(self):
        self.assertFalse(self.verify(['Quota', '1'], [], clock=(0, 0)))
        self.suite.request.assert_not_called()
