"""Exact local HTTP accounting checks; independent Decimal oracle, no live pricing."""
from decimal import Decimal, ROUND_HALF_UP

D = lambda value: Decimal(str(value))
QUOTA_PER_UNIT = D(500000)
VIDEO_MODEL = 'doubao-seedance-2-5-260628-mock-success'
CHAT_MODELS = ['mock-chat', 'claude-sonnet-4-5', 'claude-opus-5-5', 'gemini-2.5-flash']
IMAGE_MODELS = ['gpt-image-2', 'image-alias']
exact_results = []


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
            expression_value += D(5) * D('.4') + D(10) * 3
            formula += ' + 5 × 0.4 + 10 × 3'
        return rounded(expression_value / 1000000 * QUOTA_PER_UNIT * group), f'round(({formula}) ÷ 1000000 × 500000 × {group})'
    # Sonnet 4 has a locked output ratio of 5 in the current application contract.
    output_ratio = D(5 if model == 'claude-sonnet-4-5' else profile['completion'])
    raw = D(100) + D(20) * output_ratio
    formula = f'100 + 20 × {output_ratio}'
    if is_claude:
        raw += D(5) * D(profile['cache']) + D(10) * D(profile['creation'])
        formula += f" + 5 × {profile['cache']} + 10 × {profile['creation']}"
    return rounded(raw * D(profile['model_ratio']) * group), f"round(({formula}) × {profile['model_ratio']} × {group})"


def account_snapshot():
    row = db('SELECT u.quota AS user_quota,u.used_quota AS user_used,t.remain_quota AS token_remaining,t.used_quota AS token_used,(SELECT COALESCE(MAX(id),0) FROM logs WHERE user_id=u.id) AS last_log FROM users u JOIN tokens t ON t.user_id=u.id WHERE u.id=? AND t.id=?', [user_id, token_id])[0]
    return {key: int(value) for key, value in row.items()}


def ledger_after(before):
    rows = db('SELECT id,type,quota,prompt_tokens,completion_tokens,model_name,other FROM logs WHERE user_id=? AND id>? AND type IN (2,6) ORDER BY id', [user_id, before['last_log']])
    for row in rows:
        for key in ['id', 'type', 'quota', 'prompt_tokens', 'completion_tokens']:
            row[key] = int(row[key])
        row['other'] = json.loads(row['other']) if isinstance(row['other'], str) else row['other']
    return rows


def verify_charge(name, before, expected, formula, *, model=None, log_count=None, per_log=None):
    deadline = time.monotonic() + 6
    while True:
        after = account_snapshot()
        rows = ledger_after(before)
        raw_cache = redis_cmd('HGETALL', 'user:' + str(user_id))
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
        ok = all(value == expected for value in actual.values()) and cache_ok and log_ok
        if ok or time.monotonic() >= deadline:
            break
        time.sleep(.05)
    entry = {'name': name, 'expected_quota': expected, 'formula': formula, 'actual': actual, 'redis_matches_mysql': cache_ok, 'log_count': len(rows), 'expected_log_count': log_count, 'logs_valid': log_ok, 'passed': ok}
    exact_results.append(entry)
    (OUT/'exact-accounting.json').write_text(json.dumps(exact_results, ensure_ascii=False, indent=2))
    (OUT/'private'/f'ledger-{len(exact_results):03}.json').write_text(json.dumps({'before': before, 'after': after, 'logs': rows}, ensure_ascii=False, indent=2))
    check('exact:' + name, ok, **{k: v for k, v in entry.items() if k not in ('name', 'passed')})
    return after


def configure_profile(profile):
    target_models = CHAT_MODELS + IMAGE_MODELS + [VIDEO_MODEL]
    ratios = {m: 1 for m in models}
    ratios.update({m: float(profile['model_ratio']) for m in target_models})
    # Mapped upstream names deliberately have different prices: the client model
    # is the billing model for these aliases.
    ratios['gpt-4o'] = 9
    ratios['gpt-image-2.6-sunburst'] = 11
    option('ModelRatio', ratios)
    option('CompletionRatio', {m: float(profile['completion']) for m in models})
    option('CacheRatio', {m: float(profile['cache']) for m in CHAT_MODELS})
    option('CreateCacheRatio', {m: float(profile['creation']) for m in CHAT_MODELS})
    option('GroupRatio', {'default': float(profile['group'])})
    prices = {} if profile['mode'] == 'tiered' else {VIDEO_MODEL: float(profile['video_price'])}
    if profile['mode'] == 'price':
        prices.update({m: float(profile['image_price']) for m in IMAGE_MODELS})
    option('ModelPrice', prices)
    exprs = {m: 'tier("exact", p * 2 + c * 8 + cr * 0.4 + cc * 3)' for m in target_models} if profile['mode'] == 'tiered' else {}
    option('billing_setting.billing_expr', exprs)
    option('billing_setting.billing_mode', {m: 'tiered_expr' for m in exprs})
    stored = {r['key']: r['value'] for r in db('SELECT `key`,value FROM options WHERE `key` IN (?,?,?,?,?,?,?,?)', ['ModelRatio', 'CompletionRatio', 'CacheRatio', 'CreateCacheRatio', 'GroupRatio', 'ModelPrice', 'billing_setting.billing_expr', 'billing_setting.billing_mode'])}
    (OUT/'private'/('pricing-' + profile['name'] + '.json')).write_text(json.dumps(stored, ensure_ascii=False, indent=2))


def chat_case(profile, model, stream=False, native=False):
    label = profile['name'] + '/chat/' + model + ('/native' if native else '/openai') + ('/stream' if stream else '/json')
    before = account_snapshot()
    if native and model.startswith('gemini'):
        path = '/v1beta/models/' + model + (':streamGenerateContent?alt=sse' if stream else ':generateContent')
        body = {'contents': [{'role': 'user', 'parts': [{'text': 'exact billing'}]}]}
    else:
        path = '/v1/messages' if native else '/v1/chat/completions'
        body = {'model': model, 'messages': [{'role': 'user', 'content': 'exact billing'}], 'max_tokens': 128, 'stream': stream}
        if stream and not native:
            body['stream_options'] = {'include_usage': True}
    response = call_check('exact-http:' + label, path, body)
    check('exact-content:' + label, 'mock ' in response.text)
    expected, formula = expected_charge(model, profile)
    verify_charge(label, before, expected, formula, model=model, log_count=1, per_log=expected)


def image_case(profile, model, n=1, asynchronous=False, edit=False):
    label = profile['name'] + '/image/' + model + ('/async' if asynchronous else '/edit' if edit else '/sync') + '/n=' + str(n)
    before = account_snapshot()
    body = {'model': model, 'prompt': 'MOCK_HOLD exact image' if asynchronous else 'exact image', 'n': n, 'response_format': 'b64_json', 'size': '1024x1024'}
    if edit:
        response = multipart_request('/v1/images/edits', {k: str(v) for k, v in body.items()}, {'image': ('input.png', base64.b64decode(PNG), 'image/png')})
        check('exact-http:' + label, response.status_code == 200, status=response.status_code)
    else:
        response = call_check('exact-http:' + label, '/v1/images/generations' + ('?async=true' if asynchronous else ''), body)
    ident = None
    if asynchronous and response.status_code == 200:
        ident = response.json().get('id')
        check('exact-task-created:' + label, bool(ident))
        if ident:
            deadline = time.monotonic() + 15
            while True:
                task_response = request('/v1/tasks/' + ident, auth=token)
                task = task_response.json()
                if task.get('state') in ('succeeded', 'failed') or time.monotonic() > deadline:
                    break
                time.sleep(.1)
            check('exact-task-terminal:' + label, task.get('state') == 'succeeded', state=task.get('state'))
    elif response.status_code == 200:
        check('exact-content:' + label, len(response.json().get('data', [])) == n)
    expected, formula = expected_charge(model, profile, n)
    verify_charge(label, before, expected, formula, model=model, log_count=1, per_log=expected)
    if ident:
        rows = db('SELECT quota,status FROM tasks WHERE task_id=?', [ident])
        check('exact-task-quota:' + label, len(rows) == 1 and int(rows[0]['quota']) == expected, expected=expected, rows=rows)
        if profile['name'] == 'baseline' and n == 1:
            before = account_snapshot()
            load('exact-image-repeat', 20, 10, lambda i: request('/v1/tasks/' + ident, auth=token))
            verify_charge(label + '/repeat', before, 0, 'completed task queried 20 times: 0', log_count=0)
        return ('image', '/v1/tasks/' + ident)


def video_create_body():
    return {'model': VIDEO_MODEL, 'content': [{'type': 'text', 'text': 'exact video billing'}], 'duration': -1, 'seed': 0, 'return_last_frame': False}


def video_case(profile):
    label = profile['name'] + '/video/success'
    before = account_snapshot()
    response = call_check('exact-http:' + label, '/api/v3/contents/generations/tasks', video_create_body())
    ident = response.json().get('id') if response.status_code == 200 else None
    check('exact-task-created:' + label, bool(ident))
    if ident:
        response = request('/api/v3/contents/generations/tasks/' + ident, auth=token)
        check('exact-task-terminal:' + label, response.status_code == 200 and response.json().get('status') == 'succeeded', status=response.status_code, body=response.json())
    expected, formula = expected_charge(VIDEO_MODEL, profile)
    verify_charge(label, before, expected, formula, model=VIDEO_MODEL, log_count=1 if profile['mode'] != 'tiered' else None)
    if ident:
        rows = db('SELECT quota,status FROM tasks WHERE private_data LIKE ?', ['%' + ident + '%'])
        check('exact-task-quota:' + label, len(rows) == 1 and int(rows[0]['quota']) == expected, expected=expected, rows=rows)
        before = account_snapshot()
        load('exact-video-repeat-' + profile['name'], 20, 10, lambda i: request('/api/v3/contents/generations/tasks/' + ident, auth=token))
        verify_charge(label + '/repeat', before, 0, 'completed task queried 20 times: 0', log_count=0)
        return ('video', '/api/v3/contents/generations/tasks/' + ident)


def concurrent_cases(profile):
    for model, count in [('mock-chat', 100), ('claude-sonnet-4-5', 50), ('gemini-2.5-flash', 50), ('gpt-image-2', 20)]:
        image = model == 'gpt-image-2'
        path = '/v1/images/generations' if image else '/v1/chat/completions'
        body = {'model': model, 'prompt': 'exact concurrent', 'n': 1, 'response_format': 'b64_json'} if image else {'model': model, 'messages': [{'role': 'user', 'content': 'exact concurrent'}], 'max_tokens': 128}
        before = account_snapshot()
        rows = load('exact-concurrent-' + model, count, count, lambda i: request(path, 'POST', token, body))
        check('exact-concurrent-success:' + model, all(r['status'] == 200 for r in rows))
        expected, formula = expected_charge(model, profile)
        verify_charge(profile['name'] + '/concurrent/' + model, before, expected * count, f'{count} × ({formula})', model=model, log_count=count, per_log=expected)
    before = account_snapshot()
    created = load('exact-video-concurrent-create', 10, 10, lambda i: request('/api/v3/contents/generations/tasks', 'POST', token, video_create_body()))
    ids = [r.get('body', {}).get('id') for r in created if r['status'] == 200]
    check('exact-video-concurrent-created', len(ids) == 10 and len(set(ids)) == 10 and all(ids))
    if len(ids) == 10 and all(ids):
        load('exact-video-concurrent-fetch', 10, 10, lambda i: request('/api/v3/contents/generations/tasks/' + ids[i], auth=token))
    expected, formula = expected_charge(VIDEO_MODEL, profile)
    verify_charge(profile['name'] + '/concurrent/video', before, expected * 10, f'10 × ({formula})', model=VIDEO_MODEL, log_count=10, per_log=expected)


def test_exact_billing():
    profiles = [
        {'name': 'baseline', 'mode': 'ratio', 'group': '1', 'model_ratio': '1', 'completion': '1', 'cache': '1', 'creation': '1.25', 'video_price': '.001'},
        {'name': 'fractional', 'mode': 'ratio', 'group': '.65', 'model_ratio': '1.7', 'completion': '2.3', 'cache': '.2', 'creation': '1.4', 'video_price': '.0017'},
        {'name': 'per_call', 'mode': 'price', 'group': '.65', 'model_ratio': '1', 'completion': '1', 'cache': '1', 'creation': '1.25', 'video_price': '.0017', 'image_price': '.0027'},
        {'name': 'tiered', 'mode': 'tiered', 'group': '.65', 'model_ratio': '1.7', 'completion': '2.3', 'cache': '.2', 'creation': '1.4', 'video_price': '.0017'},
    ]
    completed = []
    for profile in profiles:
        print('Exact accounting profile: ' + profile['name'], flush=True)
        configure_profile(profile)
        if profile['mode'] != 'price':
            for model in CHAT_MODELS:
                for stream in [False, True]:
                    chat_case(profile, model, stream)
            for model in ['claude-sonnet-4-5', 'gemini-2.5-flash']:
                for stream in [False, True]:
                    chat_case(profile, model, stream, native=True)
        for model in IMAGE_MODELS:
            for n in [1, 3]:
                image_case(profile, model, n)
        image_case(profile, 'gpt-image-2', edit=True)
        for n in [1, 3]:
            result = image_case(profile, 'gpt-image-2', n, asynchronous=True)
            if result: completed.append(result)
        result = video_case(profile)
        if result: completed.append(result)
        if profile['name'] == 'baseline':
            concurrent_cases(profile)
    for name, price in [('integer_boundary_350x07', '.0007'), ('integer_boundary_1350x07', '.0027')]:
        profile = {**profiles[0], 'name': name, 'group': '.7', 'video_price': price}
        configure_profile(profile)
        result = video_case(profile)
        if result: completed.append(result)
    before = account_snapshot()
    stop_app(); start_app()
    for kind, path in completed:
        response = request(path, auth=token)
        check('exact-restart-query:' + kind, response.status_code == 200, path=path, status=response.status_code)
    verify_charge('restart/all_completed_tasks', before, 0, 'restart and query completed tasks from all profiles: 0', log_count=0)
    check('exact-all-accounting-cases', bool(exact_results) and all(r['passed'] for r in exact_results), cases=len(exact_results), failures=[r['name'] for r in exact_results if not r['passed']])
