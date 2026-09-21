import concurrent.futures
import http.cookies
import json
import os
import statistics
import subprocess
import sys
import threading
import time
import urllib.error
import urllib.request
from pathlib import Path

BASE = Path('/opt/apimeter-server/compat-20260913')
STATE_PATH = BASE / 'state.json'
state = json.loads(STATE_PATH.read_text())
PREFIX = state['prefix']
HOSTS = {}

def origin(node):
    if node not in HOSTS:
        d = json.loads(command(['docker', 'inspect', PREFIX + '-' + node]))[0]
        ip = d['NetworkSettings']['Networks'][PREFIX]['IPAddress']
        if not ip:
            raise RuntimeError('Test container has not started')
        HOSTS[node] = 'http://' + ip + ':' + str(34101 if node == 'old' else 34102)
    return HOSTS[node]

def command(args, data=None):
    return subprocess.run(args, input=data, stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=True).stdout.decode()

def sql(query, db='modelsell'):
    return command(['docker', 'exec', '-i', PREFIX + '-db', 'sh', '-c', 'exec mysql -uroot -p"$MYSQL_ROOT_PASSWORD" --batch --skip-column-names "$1"', 'sh', db], query.encode())

def save_state():
    STATE_PATH.write_text(json.dumps(state, indent=2))

def request(node, path, method='GET', payload=None, headers=None, timeout=15):
    started = time.monotonic()
    req_headers = {'Content-Type': 'application/json', **(headers or {})}
    try:
        req = urllib.request.Request(origin(node) + path, data=json.dumps(payload).encode() if payload is not None else None, headers=req_headers, method=method)
        response = urllib.request.urlopen(req, timeout=timeout)
    except urllib.error.HTTPError as err:
        response = err
    except Exception as err:
        return {'status': 0, 'ok': False, 'error': type(err).__name__ + ': ' + str(err), 'elapsed': time.monotonic() - started}, None, {}
    try:
        with response:
            status = response.status
            raw = response.read().decode(errors='replace')
            response_headers = dict(response.headers)
            response_headers['_cookies'] = response.headers.get_all('Set-Cookie') or []
    except Exception as err:
        return {'status': 0, 'ok': False, 'error': type(err).__name__ + ': ' + str(err), 'elapsed': time.monotonic() - started}, None, {}
    try:
        body = json.loads(raw)
    except ValueError:
        body = None
    ok = 200 <= status < 300 and (not isinstance(body, dict) or (body.get('success') is not False and 'error' not in body))
    if payload and payload.get('stream'):
        ok = ok and '[DONE]' in raw and 'compat-ok' in raw
    elif path == '/v1/chat/completions' and ok:
        ok = (body or {}).get('choices', [{}])[0].get('message', {}).get('content') == 'compat-ok'
    result = {'status': status, 'ok': ok, 'elapsed': round(time.monotonic() - started, 4)}
    if not ok:
        result['error'] = str((body or {}).get('error') or (body or {}).get('message') or raw[:240])[:450]
    return result, body, response_headers

def login(node, username):
    result, body, resp_headers = request(node, '/api/user/login', 'POST', {'username': username, 'password': state['password']})
    print('LOGIN', node, username, result, 'data_keys', list((body or {}).get('data', {})), flush=True)
    if not result['ok']:
        return None
    uid = state['users'][username]
    headers = {'New-Api-User': str(uid)}
    cookies = http.cookies.SimpleCookie()
    for cookie in resp_headers['_cookies']:
        cookies.load(cookie)
    headers['Cookie'] = '; '.join(k + '=' + v.value for k, v in cookies.items())
    access = body.get('data', {}).get('access_token')
    if access:
        headers['Authorization'] = 'Bearer ' + access
    state.setdefault('sessions', {})[node + ':' + username] = headers
    save_state()
    return headers

def check(label, response):
    result = response[0]
    print(label, json.dumps(result, ensure_ascii=False), flush=True)
    return result

def seed():
    for _ in range(60):
        result, body, _ = request('old', '/api/status')
        if result['ok']:
            print('OLD VERSION', body.get('data', {}).get('version'), flush=True)
            break
        time.sleep(1)
    else:
        raise SystemExit('Old slave did not start')
    state.setdefault('users', {})
    for username, role in [('compatroot', 100), ('compatuser', 1), ('compatrevoke', 1)]:
        uid = sql("SELECT id FROM users WHERE username='" + username + "'").strip()
        if not uid:
            response = request('old', '/api/user/register', 'POST', {'username': username, 'password': state['password']})
            if not check('REGISTER ' + username, response)['ok']:
                raise SystemExit('Cannot seed test user')
            uid = sql("SELECT id FROM users WHERE username='" + username + "'").strip()
        state['users'][username] = int(uid)
        sql(f"UPDATE users SET role={role},status=1,quota=100000000,used_quota=0,request_count=0 WHERE id={int(uid)};")
    save_state()
    root = login('old', 'compatroot')
    user = login('old', 'compatuser')
    revoke = login('old', 'compatrevoke')
    if not all([root, user, revoke]):
        raise SystemExit('Cannot log into old slave')
    channel = {'mode': 'single', 'channel': {'type': 1, 'name': 'compat-mock', 'key': 'mock-only', 'base_url': 'http://mock:8080', 'models': 'gpt-3.5-turbo,dall-e-3', 'group': 'default', 'status': 1, 'weight': 1, 'channel_ratio': 1}}
    if not sql("SELECT id FROM channels WHERE name='compat-mock'").strip():
        if not check('CREATE CHANNEL', request('old', '/api/channel/', 'POST', channel, root))['ok']:
            raise SystemExit('Cannot create mock channel')
    state['channel_id'] = int(sql("SELECT id FROM channels WHERE name='compat-mock' ORDER BY id DESC LIMIT 1").strip())
    for username, headers in [('compatuser', user), ('compatrevoke', revoke)]:
        uid = state['users'][username]
        token_name = username + '-key'
        if not sql(f"SELECT id FROM tokens WHERE user_id={uid} AND name='{token_name}'").strip():
            if not check('CREATE TOKEN ' + username, request('old', '/api/token/', 'POST', {'name': token_name, 'expired_time': -1, 'remain_quota': 100000000, 'unlimited_quota': False, 'group': 'default'}, headers))['ok']:
                raise SystemExit('Cannot create test token')
        row = sql(f"SELECT id, `key` FROM tokens WHERE user_id={uid} AND name='{token_name}' ORDER BY id DESC LIMIT 1").strip().split('\t')
        state.setdefault('tokens', {})[username] = {'id': int(row[0]), 'key': row[1]}
    save_state()
    time.sleep(6)
    for path in ['/api/user/self', '/api/token/?p=1&page_size=10', '/api/log/self?p=1&page_size=10']:
        check('BASELINE ' + path, request('old', path, headers=user))
    key = {'Authorization': 'Bearer sk-' + state['tokens']['compatuser']['key']}
    check('BASELINE chat', request('old', '/v1/chat/completions', 'POST', {'model': 'gpt-3.5-turbo', 'messages': [{'role': 'user', 'content': 'compat test'}]}, key))
    check('BASELINE sse', request('old', '/v1/chat/completions', 'POST', {'model': 'gpt-3.5-turbo', 'messages': [{'role': 'user', 'content': 'compat test'}], 'stream': True, 'stream_options': {'include_usage': True}}, key))

if __name__ == '__main__':
    if sys.argv[1] == 'seed':
        seed()
