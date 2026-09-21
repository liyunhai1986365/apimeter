import collections
import concurrent.futures
import json
import math
import threading
import time
from pathlib import Path
import client as c

BASE = c.BASE
phase = 'baseline'
stop = threading.Event()
lock = threading.Lock()
events = []
special = {}
workers = []
key = {'Authorization': 'Bearer sk-' + c.state['tokens']['compatuser']['key']}
old_user = c.state['sessions']['old:compatuser']
chat = {'model': 'gpt-3.5-turbo', 'messages': [{'role': 'user', 'content': 'compat exercise'}]}
event_file = (BASE / 'requests.jsonl').open('w')

def record(node, operation, result, started, request_phase):
    row = {'node': node, 'operation': operation, 'phase': request_phase, 'started': started, 'finished': time.time(), **result}
    with lock:
        events.append(row)
        event_file.write(json.dumps(row, ensure_ascii=False) + '\n')
        event_file.flush()
    return result

def call(node, operation, path, method='GET', payload=None, headers=None, timeout=15):
    started, request_phase = time.time(), phase
    response = c.request(node, path, method, payload, headers, timeout)
    record(node, operation, response[0], started, request_phase)
    return response

def repeat(node, operation, path, interval, method='GET', payload=None, headers=None):
    while not stop.is_set():
        begin = time.monotonic()
        try:
            call(node, operation, path, method, payload, headers)
        except Exception as err:
            record(node, operation, {'ok': False, 'status': 0, 'elapsed': time.monotonic() - begin, 'error': repr(err)}, time.time(), phase)
        stop.wait(max(0.01, interval - (time.monotonic() - begin)))

def worker(*args, **kwargs):
    t = threading.Thread(target=repeat, args=args, kwargs=kwargs, daemon=True)
    t.start()
    workers.append(t)

def wait_period(seconds):
    end = time.monotonic() + seconds
    while time.monotonic() < end:
        time.sleep(min(15, end - time.monotonic()))
        with lock:
            rows = [e for e in events if e['phase'] == phase]
            failures = sum(not e['ok'] for e in rows)
        print('PROGRESS', phase, 'requests', len(rows), 'failures', failures, flush=True)

def snapshot():
    uid = c.state['users']['compatuser']
    tid = c.state['tokens']['compatuser']['id']
    return {'user': c.sql(f'SELECT quota,used_quota,request_count FROM users WHERE id={uid}').strip(), 'token': c.sql(f'SELECT remain_quota,used_quota,status FROM tokens WHERE id={tid}').strip(), 'logs': c.sql(f'SELECT type,COUNT(*),COALESCE(SUM(quota),0) FROM logs WHERE user_id={uid} GROUP BY type', 'modelsell-log').strip(), 'log_max_id': c.sql('SELECT MAX(id) FROM logs', 'modelsell-log').strip()}

def save_schema(name):
    for db in ['modelsell', 'modelsell-log']:
        schema = c.command(['docker', 'exec', c.PREFIX + '-db', 'sh', '-c', 'exec mysqldump -uroot -p"$MYSQL_ROOT_PASSWORD" --no-data --skip-comments --skip-lock-tables "$1"', 'sh', db])
        (BASE / (name + '-' + db + '-schema.sql')).write_text(schema)

def targeted(label, node, path, method='GET', payload=None, headers=None):
    response = c.request(node, path, method, payload, headers)
    special[label] = response[0]
    print('CHECK', label, json.dumps(response[0], ensure_ascii=False), flush=True)
    return response

def main():
    global phase
    before = snapshot()
    save_schema('before')
    original_names = [r['Names'] for r in json.loads((BASE / 'existing-containers.json').read_text())]
    originals = json.loads(c.command(['docker', 'inspect', *original_names]))
    protected = {r['Name']: {'id': r['Id'], 'started_at': r['State']['StartedAt'], 'status': r['State']['Status'], 'restarts': r['RestartCount']} for r in originals}
    (BASE / 'protected-before.json').write_text(json.dumps(protected, indent=2))
    worker('old', 'chat', '/v1/chat/completions', 0.5, 'POST', chat, key)
    worker('old', 'sse', '/v1/chat/completions', 1.0, 'POST', {**chat, 'stream': True, 'stream_options': {'include_usage': True}}, key)
    worker('old', 'self', '/api/user/self', 1.0, headers=old_user)
    worker('old', 'logs', '/api/log/self?p=1&page_size=10', 2.0, headers=old_user)
    worker('old', 'tokens', '/api/token/?p=1&page_size=10', 3.0, headers=old_user)
    wait_period(60)
    if any(not e['ok'] for e in events):
        raise RuntimeError('Baseline contains failures; migration not started')
    with concurrent.futures.ThreadPoolExecutor(max_workers=2) as pool:
        long_future = pool.submit(call, 'old', 'long_sse', '/v1/chat/completions', 'POST', {**chat, 'messages': [{'role': 'user', 'content': 'MOCK_LONG'}], 'stream': True, 'stream_options': {'include_usage': True}}, key, 150)
        image = targeted('old_async_image_submit', 'old', '/v1/images/generations?async=true', 'POST', {'model': 'dall-e-3', 'prompt': 'isolated compatibility test', 'n': 1, 'size': '1024x1024', 'response_format': 'b64_json'}, key)
        task_id = (image[1] or {}).get('id')
        special['old_async_image_task_created'] = bool(task_id)
        phase = 'migration'
        started = time.time()
        c.command(['docker', 'start', c.PREFIX + '-new'])
        print('NEW MASTER STARTED', started, flush=True)
        ready = False
        for _ in range(180):
            result = c.request('new', '/api/ready', timeout=3)[0]
            if result['ok']:
                ready = True
                break
            time.sleep(1)
        special['new_ready'] = {'ok': ready, 'elapsed': time.time() - started, 'started': started, 'finished': time.time()}
        print('NEW MASTER READINESS', special['new_ready'], flush=True)
        if not ready:
            raise RuntimeError('New master failed readiness')
        phase = 'mixed'
        targeted('old_cookie_on_new', 'new', '/api/user/self', headers=old_user)
        new_root = c.login('new', 'compatroot')
        new_user = c.login('new', 'compatuser')
        new_revoke = c.login('new', 'compatrevoke')
        revoke_key = {'Authorization': 'Bearer sk-' + c.state['tokens']['compatrevoke']['key']}
        if new_user:
            targeted('new_session_on_old', 'old', '/api/user/self', headers=new_user)
            worker('new', 'chat', '/v1/chat/completions', 1.0, 'POST', chat, key)
            worker('new', 'self', '/api/user/self', 1.0, headers=new_user)
        if task_id:
            time.sleep(12)
            for node in ['old', 'new']:
                response = targeted(node + '_read_old_image_task', node, '/v1/tasks/' + task_id, headers=key)
                body = response[1] or {}
                special[node + '_image_task_state'] = body.get('state') or body.get('status')
        if new_revoke:
            revoke_key = {'Authorization': 'Bearer sk-' + c.state['tokens']['compatrevoke']['key']}
            targeted('old_revoke_token_warmup', 'old', '/v1/chat/completions', 'POST', chat, revoke_key)
            tid = c.state['tokens']['compatrevoke']['id']
            targeted('new_disable_token', 'new', '/api/token/?status_only=true', 'PUT', {'id': tid, 'status': 2}, new_revoke)
            time.sleep(6)
            targeted('old_reject_disabled_token', 'old', '/v1/chat/completions', 'POST', chat, revoke_key)
            targeted('new_enable_token', 'new', '/api/token/?status_only=true', 'PUT', {'id': tid, 'status': 1}, new_revoke)
            time.sleep(6)
            targeted('old_accept_enabled_token', 'old', '/v1/chat/completions', 'POST', chat, revoke_key)
        if new_root:
            uid = c.state['users']['compatrevoke']
            targeted('new_disable_user', 'new', '/api/user/manage', 'POST', {'id': uid, 'action': 'disable'}, new_root)
            time.sleep(6)
            targeted('old_cookie_disabled_user', 'old', '/api/user/self', headers=c.state['sessions']['old:compatrevoke'])
            targeted('old_token_disabled_user', 'old', '/v1/chat/completions', 'POST', chat, revoke_key)
            targeted('new_enable_user', 'new', '/api/user/manage', 'POST', {'id': uid, 'action': 'enable'}, new_root)
        wait_period(120)
        special['long_sse'] = long_future.result()[0]
    stop.set()
    for t in workers:
        t.join(timeout=20)
    time.sleep(8)
    after = snapshot()
    save_schema('after')
    phase = 'old_after_new_stopped'
    c.command(['docker', 'stop', c.PREFIX + '-new'])
    for path in ['/api/user/self', '/api/log/self?p=1&page_size=10', '/api/token/?p=1&page_size=10']:
        targeted('new_stopped:' + path, 'old', path, headers=old_user)
    targeted('new_stopped:chat', 'old', '/v1/chat/completions', 'POST', chat, key)
    c.command(['docker', 'restart', c.PREFIX + '-old'])
    c.HOSTS.pop('old', None)
    for _ in range(60):
        if c.request('old', '/api/status')[0]['ok']:
            break
        time.sleep(1)
    for path in ['/api/user/self', '/api/log/self?p=1&page_size=10', '/api/token/?p=1&page_size=10']:
        targeted('old_restarted:' + path, 'old', path, headers=old_user)
    targeted('old_restarted:chat', 'old', '/v1/chat/completions', 'POST', chat, key)
    for node in ['old', 'new']:
        p = __import__('subprocess').run(['docker', 'logs', '--timestamps', c.PREFIX + '-' + node], stdout=__import__('subprocess').PIPE, stderr=__import__('subprocess').STDOUT)
        (BASE / (node + '-app.log')).write_bytes(p.stdout)
    grouped = {}
    for group in sorted(set((e['phase'], e['node'], e['operation']) for e in events)):
        rows = [e for e in events if (e['phase'], e['node'], e['operation']) == group]
        times = sorted(e['elapsed'] for e in rows)
        grouped['/'.join(group)] = {'count': len(rows), 'failures': sum(not e['ok'] for e in rows), 'p95_seconds': times[max(0, math.ceil(len(times)*0.95)-1)], 'max_seconds': max(times)}
    originals_after = json.loads(c.command(['docker', 'inspect', *original_names]))
    protected_after = {r['Name']: {'id': r['Id'], 'started_at': r['State']['StartedAt'], 'status': r['State']['Status'], 'restarts': r['RestartCount']} for r in originals_after}
    report = {'images': c.state['images'], 'before': before, 'after': after, 'groups': grouped, 'checks': special, 'failures': [e for e in events if not e['ok']], 'existing_containers_unchanged': protected == protected_after, 'protected_after': protected_after}
    (BASE / 'result.json').write_text(json.dumps(report, ensure_ascii=False, indent=2))
    print('COMPLETE', json.dumps({'requests': len(events), 'failures': len(report['failures']), 'existing_containers_unchanged': report['existing_containers_unchanged'], 'result': str(BASE / 'result.json')}), flush=True)

if __name__ == '__main__':
    try:
        main()
    finally:
        stop.set()
        for t in workers:
            t.join(timeout=20)
        event_file.close()
