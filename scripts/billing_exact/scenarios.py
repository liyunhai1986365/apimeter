"""Exact local HTTP accounting checks; independent Decimal oracle, no live pricing."""
import base64
import json
import time
from decimal import Decimal, ROUND_HALF_UP
from urllib.parse import urlsplit

from requests.exceptions import RequestException

from upstream import PNG

D = lambda value: Decimal(str(value))
QUOTA_PER_UNIT = D(500000)
EXACT_CASE_COUNT = 88
VIDEO_MODEL = 'doubao-seedance-2-5-260628-mock-success'
CHAT_MODELS = ['mock-chat', 'claude-sonnet-4-5', 'claude-opus-5-5', 'gemini-2.5-flash']
IMAGE_MODELS = ['gpt-image-2', 'image-alias']
CONCURRENT_CHAT_TEXT = {
    'mock-chat': 'mock openai answer',
    'claude-sonnet-4-5': 'mock claude answer',
    'gemini-2.5-flash': 'mock gemini answer',
}


def rounded(value):
    return int(value.quantize(D(1), rounding=ROUND_HALF_UP))


def expected_charge(model, profile, n=1):
    group = D(profile['group'])
    if model == VIDEO_MODEL and profile['mode'] != 'tiered':
        value = D(profile['video_price']) * QUOTA_PER_UNIT * group
        return int(value), f"truncate({profile['video_price']} × 500000 × {group})"
    if model in IMAGE_MODELS and profile['mode'] == 'price':
        value = D(profile['image_price']) * QUOTA_PER_UNIT * group * n
        return rounded(value), f"round({profile['image_price']} × 500000 × {group} × {n})"
    is_claude = model.startswith('claude-')
    if profile['mode'] == 'tiered':
        p, c = (0, 100) if model == VIDEO_MODEL else (100, 20)
        expression_value = D(p) * 2 + D(c) * 8
        formula = f'{p} × 2 + {c} × 8'
        if is_claude:
            expression_value += D(5) * D('.4') + D(3) * 3 + D(7) * 8
            formula += ' + 5 × 0.4 + 3 × 3 + 7 × 8'
        return rounded(expression_value / 1000000 * QUOTA_PER_UNIT * group), f'round(({formula}) ÷ 1000000 × 500000 × {group})'
    # Sonnet 4 has a locked output ratio of 5 in the current application contract.
    output_ratio = D(5 if model == 'claude-sonnet-4-5' else profile['completion'])
    raw = D(100) + D(20) * output_ratio
    formula = f'100 + 20 × {output_ratio}'
    if is_claude:
        long_ratio = D(profile.get('creation_1h', D(profile['creation']) * D('1.6')))
        raw += D(5) * D(profile['cache']) + D(3) * D(profile['creation']) + D(7) * long_ratio
        formula += f" + 5 × {profile['cache']} + 3 × {profile['creation']} + 7 × {long_ratio}"
    return rounded(raw * D(profile['model_ratio']) * group), f"round(({formula}) × {profile['model_ratio']} × {group})"


class ExactBillingCases:
    def check_upstream_model(self, name, start, path, expected):
        with self.upstream.lock:
            observed = [r['model'] for r in self.upstream.records[start:] if r['path'].endswith(path)]
        self.check('exact-upstream-model:' + name, bool(observed) and all(m == expected for m in observed),
                   expected=expected, observed=observed)

    def images_match(self, images, count):
        if not isinstance(images, list) or len(images) != count:
            return False
        for image in images:
            if not isinstance(image, dict):
                return False
            if image.get('b64_json') == PNG:
                continue
            url = image.get('url')
            if not isinstance(url, str):
                return False
            asset = urlsplit(url)
            base = urlsplit(self.base)
            if ((asset.scheme or asset.netloc) and (asset.scheme, asset.netloc) != (base.scheme, base.netloc)
                    or not asset.path.startswith('/api/relay-temp-images/')):
                return False
            response = self.request(asset.path)
            if response.status_code != 200 or response.content != base64.b64decode(PNG):
                return False
        return True

    def concurrent_result_matches(self, model, row):
        body = row.get('body')
        if row.get('status') != 200 or not isinstance(body, dict) or body.get('error') is not None:
            return False
        if model in IMAGE_MODELS:
            return self.images_match(body.get('data'), 1)
        choices = body.get('choices')
        if body.get('object') != 'chat.completion' or not isinstance(choices, list) or len(choices) != 1:
            return False
        choice = choices[0]
        if not isinstance(choice, dict) or choice.get('finish_reason') != 'stop':
            return False
        message = choice.get('message')
        return (isinstance(message, dict) and message.get('role') == 'assistant'
                and message.get('content') == CONCURRENT_CHAT_TEXT[model])

    def account_snapshot(self):
        row = self.db('SELECT u.quota AS user_quota,u.used_quota AS user_used,t.remain_quota AS token_remaining,t.used_quota AS token_used,(SELECT COALESCE(MAX(id),0) FROM logs WHERE user_id=u.id) AS last_log FROM users u JOIN tokens t ON t.user_id=u.id WHERE u.id=? AND t.id=?', [self.user_id, self.token_id])[0]
        return {key: int(value) for key, value in row.items()}


    def ledger_after(self, before):
        rows = self.db('SELECT id,type,quota,prompt_tokens,completion_tokens,model_name,other FROM logs WHERE user_id=? AND id>? AND type IN (2,6) ORDER BY id', [self.user_id, before['last_log']])
        for row in rows:
            for key in ['id', 'type', 'quota', 'prompt_tokens', 'completion_tokens']:
                row[key] = int(row[key])
            row['other'] = json.loads(row['other']) if isinstance(row['other'], str) else row['other']
        return rows


    def verify_charge(self, name, before, expected, formula, *, model=None, log_count=None, per_log=None):
        deadline = time.monotonic() + 6
        cache_key = f'user:{self.user_id}'
        saw_cache_mismatch = False
        while True:
            after = self.account_snapshot()
            rows = self.ledger_after(before)
            raw_cache = self.redis_cmd('HGETALL', cache_key)
            cache = dict(zip(raw_cache[::2], raw_cache[1::2]))
            actual = {
                'mysql_user_debit': before['user_quota'] - after['user_quota'],
                'mysql_user_used_delta': after['user_used'] - before['user_used'],
                'mysql_token_debit': before['token_remaining'] - after['token_remaining'],
                'mysql_token_used_delta': after['token_used'] - before['token_used'],
                'ledger_net': sum(r['quota'] if r['type'] == 2 else -r['quota'] for r in rows),
            }
            cache_ok = cache.get('Quota') == str(after['user_quota'])
            log_ok = (log_count is None or len(rows) == log_count) and (model is None or all(r['model_name'] == model for r in rows))
            if per_log is not None:
                log_ok = log_ok and all(r['type'] == 2 and r['quota'] == per_log for r in rows)
            settled = all(value == expected for value in actual.values()) and log_ok
            if (settled and (cache_ok or raw_cache == [])) or time.monotonic() >= deadline:
                break
            if settled:
                saw_cache_mismatch = True
            time.sleep(.05)
        refill_status = None
        if settled and raw_cache == [] and not saw_cache_mismatch:
            # A low-balance notification invalidates the entire user hash.
            try:
                refill_status = self.request('/v1/models', auth=self.token).status_code
            except RequestException as exc:
                refill_status = type(exc).__name__
            cache_ok = refill_status == 200 and self.redis_cmd('HGET', cache_key, 'Quota') == str(after['user_quota'])
        ok = settled and cache_ok
        entry = {'name': name, 'expected_quota': expected, 'formula': formula, 'actual': actual, 'redis_matches_mysql': cache_ok, 'log_count': len(rows), 'expected_log_count': log_count, 'logs_valid': log_ok, 'passed': ok}
        if refill_status is not None:
            entry['redis_refill_status'] = refill_status
        self.exact_results.append(entry)
        (self.OUT/'exact-accounting.json').write_text(json.dumps(self.exact_results, ensure_ascii=False, indent=2))
        (self.OUT/'private'/f'ledger-{len(self.exact_results):03}.json').write_text(json.dumps({'before': before, 'after': after, 'logs': rows}, ensure_ascii=False, indent=2))
        self.check('exact:' + name, ok, **{k: v for k, v in entry.items() if k not in ('name', 'passed')})
        return after


    def configure_profile(self, profile):
        target_models = CHAT_MODELS + IMAGE_MODELS + [VIDEO_MODEL]
        ratios = {m: 1 for m in self.models}
        ratios.update({m: float(profile['model_ratio']) for m in target_models})
        # Mapped upstream names deliberately have different prices: the client model
        # is the billing model for these aliases.
        ratios['gpt-4o'] = 9
        ratios['gpt-image-2.6-sunburst'] = 11
        self.option('ModelRatio', ratios)
        self.option('CompletionRatio', {m: float(profile['completion']) for m in self.models})
        self.option('CacheRatio', {m: float(profile['cache']) for m in CHAT_MODELS})
        self.option('CreateCacheRatio', {m: float(profile['creation']) for m in CHAT_MODELS})
        self.option('CreateCache1hRatio', {m: float(profile['creation_1h']) for m in CHAT_MODELS if m.startswith('claude-') and 'creation_1h' in profile})
        self.option('GroupRatio', {'default': float(profile['group'])})
        prices = {} if profile['mode'] == 'tiered' else {VIDEO_MODEL: float(profile['video_price'])}
        if profile['mode'] == 'price':
            prices.update({m: float(profile['image_price']) for m in IMAGE_MODELS})
        self.option('ModelPrice', prices)
        exprs = {m: 'tier("exact", p * 2 + c * 8 + cr * 0.4 + cc * 3 + cc1h * 8)' for m in target_models} if profile['mode'] == 'tiered' else {}
        self.option('billing_setting.billing_expr', exprs)
        self.option('billing_setting.billing_mode', {m: 'tiered_expr' for m in exprs})
        stored = {r['key']: r['value'] for r in self.db('SELECT `key`,value FROM options WHERE `key` IN (?,?,?,?,?,?,?,?,?)', ['ModelRatio', 'CompletionRatio', 'CacheRatio', 'CreateCacheRatio', 'CreateCache1hRatio', 'GroupRatio', 'ModelPrice', 'billing_setting.billing_expr', 'billing_setting.billing_mode'])}
        (self.OUT/'private'/('pricing-' + profile['name'] + '.json')).write_text(json.dumps(stored, ensure_ascii=False, indent=2))


    def chat_case(self, profile, model, stream=False, native=False, layout=None):
        label = profile['name'] + '/chat/' + model + ('/native' if native else '/openai') + ('/stream' if stream else '/json')
        if layout:
            label += '/' + layout
        if model == 'mock-chat':
            upstream_start = len(self.upstream.records)
        before = self.account_snapshot()
        if native and model.startswith('gemini'):
            path = '/v1beta/models/' + model + (':streamGenerateContent?alt=sse' if stream else ':generateContent')
            body = {'contents': [{'role': 'user', 'parts': [{'text': 'exact billing'}]}]}
        else:
            path = '/v1/messages' if native else '/v1/chat/completions'
            body = {'model': model, 'messages': [{'role': 'user', 'content': layout or 'exact billing'}], 'max_tokens': 128, 'stream': stream}
            if stream and not native:
                body['stream_options'] = {'include_usage': True}
        response = self.call_check('exact-http:' + label, path, body)
        if model == 'mock-chat':
            self.check_upstream_model(label, upstream_start, '/chat/completions', 'gpt-4o')
        self.check('exact-content:' + label, 'mock ' in response.text)
        if stream and model == 'mock-chat':
            data = [line[5:].strip() for line in response.text.splitlines() if line.startswith('data:')]
            reports = [u for line in data if line != '[DONE]' and (u := json.loads(line).get('usage')) is not None]
            self.check('exact-stream-usage:' + label, data[-1:] == ['[DONE]'] and
                       reports == [{'prompt_tokens': 100, 'completion_tokens': 20, 'total_tokens': 120}], reports=reports)
        expected, formula = expected_charge(model, profile)
        self.verify_charge(label, before, expected, formula, model=model, log_count=1, per_log=expected)


        if model.startswith('claude-') and profile['mode'] == 'ratio':
            other = self.ledger_after(before)[0]['other']
            self.check('cache-split-log:' + label,
                       other.get('cache_creation_tokens_5m') == 3 and other.get('cache_creation_tokens_1h') == 7
                       and D(other.get('cache_creation_ratio_1h', -1)) == D(profile['creation_1h']))

    def image_case(self, profile, model, n=1, asynchronous=False, edit=False):
        label = profile['name'] + '/image/' + model + ('/async' if asynchronous else '/edit' if edit else '/sync') + '/n=' + str(n)
        if model == 'image-alias':
            upstream_start = len(self.upstream.records)
        before = self.account_snapshot()
        body = {'model': model, 'prompt': 'MOCK_HOLD exact image' if asynchronous else 'exact image', 'n': n, 'response_format': 'b64_json', 'size': '1024x1024'}
        if edit:
            response = self.multipart_request('/v1/images/edits', {k: str(v) for k, v in body.items()}, {'image': ('input.png', base64.b64decode(PNG), 'image/png')})
            self.check('exact-http:' + label, response.status_code == 200, status=response.status_code)
        else:
            response = self.call_check('exact-http:' + label, '/v1/images/generations' + ('?async=true' if asynchronous else ''), body)
        ident = None
        if asynchronous and response.status_code == 200:
            ident = response.json().get('id')
            self.check('exact-task-created:' + label, bool(ident))
            if ident:
                deadline = time.monotonic() + 15
                while True:
                    task_response = self.request('/v1/tasks/' + ident, auth=self.token)
                    task = task_response.json()
                    if task.get('state') in ('succeeded', 'failed') or time.monotonic() > deadline:
                        break
                    time.sleep(.1)
                self.check_task_result('exact-task-terminal:' + label, 'image', ident, task_response.status_code, task, n)
        elif response.status_code == 200:
            self.check('exact-content:' + label, self.images_match(response.json().get('data'), n))
        if model == 'image-alias':
            self.check_upstream_model(label, upstream_start, '/images/edits' if edit else '/images/generations', 'gpt-image-2.6-sunburst')
        expected, formula = expected_charge(model, profile, n)
        self.verify_charge(label, before, expected, formula, model=model, log_count=1, per_log=expected)
        if ident:
            rows = self.db('SELECT quota,status FROM tasks WHERE task_id=?', [ident])
            self.check('exact-task-quota:' + label, len(rows) == 1 and int(rows[0]['quota']) == expected, expected=expected, rows=rows)
            if profile['name'] == 'baseline' and n == 1:
                self.check_repeated_task('exact-image-repeat', label, 'image', ident, '/v1/tasks/' + ident, n)
            return ('image', '/v1/tasks/' + ident, n)


    def video_create_body(self):
        return {'model': VIDEO_MODEL, 'content': [{'type': 'text', 'text': 'exact video billing'}], 'duration': -1, 'seed': 0, 'return_last_frame': False}


    def check_task_result(self, name, kind, ident, http_status, body, image_count=None):
        if not isinstance(body, dict):
            body = {}
        state = body.get({'image': 'state', 'video': 'status'}[kind])
        data = body.get('data')
        delivered = kind != 'image' or isinstance(data, dict) and self.images_match(data.get('images'), image_count)
        self.check(name, http_status == 200 and body.get('id') == ident and state == 'succeeded' and delivered,
                   expected_id=ident, returned_id=body.get('id'), state=state, http=http_status)

    def check_repeated_task(self, load_name, label, kind, ident, path, image_count=None):
        before = self.account_snapshot()
        rows = self.load(load_name, 20, 10, lambda _: self.request(path, auth=self.token))
        for index, row in enumerate(rows, 1):
            self.check_task_result(f'exact-task-repeat:{label}/{index}', kind, ident, row['status'], row['body'], image_count)
        self.verify_charge(label + '/repeat', before, 0, 'completed task queried 20 times: 0', log_count=0)


    def video_case(self, profile):
        label = profile['name'] + '/video/success'
        before = self.account_snapshot()
        response = self.call_check('exact-http:' + label, '/api/v3/contents/generations/tasks', self.video_create_body())
        ident = response.json().get('id') if response.status_code == 200 else None
        self.check('exact-task-created:' + label, bool(ident))
        if ident:
            response = self.request('/api/v3/contents/generations/tasks/' + ident, auth=self.token)
            self.check_task_result('exact-task-terminal:' + label, 'video', ident, response.status_code, response.json())
        expected, formula = expected_charge(VIDEO_MODEL, profile)
        self.verify_charge(label, before, expected, formula, model=VIDEO_MODEL, log_count=1 if profile['mode'] != 'tiered' else None)
        if ident:
            rows = self.db('SELECT quota,status FROM tasks WHERE private_data LIKE ?', ['%' + ident + '%'])
            self.check('exact-task-quota:' + label, len(rows) == 1 and int(rows[0]['quota']) == expected, expected=expected, rows=rows)
            self.check_repeated_task('exact-video-repeat-' + profile['name'], label, 'video', ident,
                                     '/api/v3/contents/generations/tasks/' + ident)
            return ('video', '/api/v3/contents/generations/tasks/' + ident, None)


    def concurrent_cases(self, profile):
        for model, count in [('mock-chat', 100), ('claude-sonnet-4-5', 50), ('gemini-2.5-flash', 50), ('gpt-image-2', 20)]:
            image = model == 'gpt-image-2'
            path = '/v1/images/generations' if image else '/v1/chat/completions'
            body = {'model': model, 'prompt': 'exact concurrent', 'n': 1, 'response_format': 'b64_json'} if image else {'model': model, 'messages': [{'role': 'user', 'content': 'exact concurrent'}], 'max_tokens': 128}
            before = self.account_snapshot()
            rows = self.load('exact-concurrent-' + model, count, count, lambda i: self.request(path, 'POST', self.token, body))
            failed = [i for i, row in enumerate(rows, 1) if not self.concurrent_result_matches(model, row)]
            self.check('exact-concurrent-success:' + model, len(rows) == count and not failed,
                       expected=count, received=len(rows), invalid_results=failed)
            expected, formula = expected_charge(model, profile)
            self.verify_charge(profile['name'] + '/concurrent/' + model, before, expected * count, f'{count} × ({formula})', model=model, log_count=count, per_log=expected)
        before = self.account_snapshot()
        created = self.load('exact-video-concurrent-create', 10, 10, lambda i: self.request('/api/v3/contents/generations/tasks', 'POST', self.token, self.video_create_body()))
        ids = [r['body'].get('id') for r in created if r['status'] == 200 and isinstance(r.get('body'), dict)]
        self.check('exact-video-concurrent-created', len(ids) == 10 and len(set(ids)) == 10 and all(ids))
        if len(ids) == 10 and all(ids):
            fetched = self.load('exact-video-concurrent-fetch', 10, 10, lambda i: self.request('/api/v3/contents/generations/tasks/' + ids[i], auth=self.token))
            for ident, row in zip(ids, fetched):
                self.check_task_result('exact-task-terminal:concurrent/video/' + ident, 'video', ident, row['status'], row['body'])
        expected, formula = expected_charge(VIDEO_MODEL, profile)
        self.verify_charge(profile['name'] + '/concurrent/video', before, expected * 10, f'10 × ({formula})', model=VIDEO_MODEL, log_count=10, per_log=expected)


    def test_exact_billing(self):
        profiles = [
            {'name': 'baseline', 'mode': 'ratio', 'group': '1', 'model_ratio': '1', 'completion': '1', 'cache': '1', 'creation': '1.25', 'creation_1h': '3.1', 'video_price': '.001'},
            {'name': 'fractional', 'creation_1h': '0', 'mode': 'ratio', 'group': '.65', 'model_ratio': '1.7', 'completion': '2.3', 'cache': '.2', 'creation': '1.4', 'video_price': '.0017'},
            {'name': 'per_call', 'mode': 'price', 'group': '.65', 'model_ratio': '1', 'completion': '1', 'cache': '1', 'creation': '1.25', 'video_price': '.0017', 'image_price': '.0027'},
            {'name': 'tiered', 'mode': 'tiered', 'group': '.65', 'model_ratio': '1.7', 'completion': '2.3', 'cache': '.2', 'creation': '1.4', 'video_price': '.0017'},
        ]
        completed = []
        for profile in profiles:
            print('Exact accounting profile: ' + profile['name'], flush=True)
            self.configure_profile(profile)
            if profile['mode'] != 'price':
                for model in CHAT_MODELS:
                    for stream in [False, True]:
                        self.chat_case(profile, model, stream)
                for model in ['claude-sonnet-4-5', 'gemini-2.5-flash']:
                    for stream in [False, True]:
                        self.chat_case(profile, model, stream, native=True)
            for model in IMAGE_MODELS:
                for n in [1, 3]:
                    self.image_case(profile, model, n)
            self.image_case(profile, 'gpt-image-2', edit=True)
            for n in [1, 3]:
                result = self.image_case(profile, 'gpt-image-2', n, asynchronous=True)
                if result: completed.append(result)
            result = self.video_case(profile)
            if result: completed.append(result)
            if profile['name'] == 'baseline':
                self.concurrent_cases(profile)
            if profile['mode'] == 'tiered':
                for layout in ['usage_before_stop', 'split_usage', 'update_prompt_and_total',
                               'update_completion', 'update_completion_and_total']:
                    self.chat_case(profile, 'mock-chat', stream=True, layout=layout)
        for name, price in [('integer_boundary_350x07', '.0007'), ('integer_boundary_1350x07', '.0027')]:
            profile = {**profiles[0], 'name': name, 'group': '.7', 'video_price': price}
            self.configure_profile(profile)
            result = self.video_case(profile)
            if result: completed.append(result)
        self.check_tasks_after_restart(completed)
        self.check('exact-all-accounting-cases', len(self.exact_results) == EXACT_CASE_COUNT and all(r['passed'] for r in self.exact_results), cases=len(self.exact_results), failures=[r['name'] for r in self.exact_results if not r['passed']])


    def check_tasks_after_restart(self, completed):
        before = self.account_snapshot()
        self.stop_app(); self.start_app()
        for kind, path, image_count in completed:
            response = self.request(path, auth=self.token)
            ident = path.rsplit('/', 1)[-1]
            self.check_task_result('exact-restart-query:' + kind, kind, ident, response.status_code, response.json(), image_count)
        self.verify_charge('restart/all_completed_tasks', before, 0, 'restart and query completed tasks from all profiles: 0', log_count=0)
