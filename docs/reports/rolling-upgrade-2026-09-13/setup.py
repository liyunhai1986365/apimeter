import json
import os
import secrets
import socket
import subprocess
import time
from pathlib import Path

BASE = Path('/opt/apimeter-server/compat-20260913')
PREFIX = 'compat0913'

def run(args, **kwargs):
    return subprocess.run(args, check=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE, **kwargs).stdout.decode()

def sql(query, database='modelsell'):
    return run(['docker', 'exec', '-i', PREFIX + '-db', 'sh', '-c', 'exec mysql -uroot -p"$MYSQL_ROOT_PASSWORD" --batch --skip-column-names "$1"', 'sh', database], input=query.encode())

def start(name, image, extra, command=()):
    return run(['docker', 'run', '-d', '--name', PREFIX + '-' + name, '--label', 'purpose=apimeter-compat-20260913', '--network', PREFIX, '--network-alias', name, '--memory', '2g', '--cpus', '2', *extra, image, *command]).strip()

def main():
    if BASE.exists():
        raise SystemExit('Test directory already exists; refusing to overwrite')
    for port in [34101, 34102]:
        with socket.socket() as s:
            s.bind(('127.0.0.1', port))
    BASE.mkdir(mode=0o700)
    baseline = json.loads(run(['docker', 'ps', '-a', '--format', 'json']).replace('\n', ',').rstrip(',').join(['[', ']']))
    (BASE / 'existing-containers.json').write_text(json.dumps(baseline, indent=2))
    run(['docker', 'network', 'create', '--internal', '--label', 'purpose=apimeter-compat-20260913', PREFIX])
    state = {'prefix': PREFIX, 'base': str(BASE), 'session_secret': secrets.token_hex(32), 'db_password': secrets.token_hex(24), 'password': secrets.token_hex(8), 'api_key': secrets.token_hex(24)}
    images = ['wagjie/modelsell-api:modelsell-2026.08.05', 'apimeter/apimeter:v1.0.2', 'mysql:5.7.43', 'redis:8-alpine', 'python:3.12-alpine']
    state['images'] = {}
    for tag in images:
        d = json.loads(run(['docker', 'image', 'inspect', tag]))[0]
        state['images'][tag] = {'id': d['Id'], 'digests': d.get('RepoDigests'), 'labels': d['Config'].get('Labels'), 'entrypoint': d['Config'].get('Entrypoint')}
    (BASE / 'state.json').write_text(json.dumps(state, indent=2))
    os.chmod(BASE / 'state.json', 0o600)
    for name in ['mysql-data', 'old-data', 'new-data']:
        (BASE / name).mkdir()
    (BASE / 'mock.py').write_bytes(Path('/tmp/apimeter-compat-mock.py').read_bytes())
    start('db', state['images']['mysql:5.7.43']['id'], ['-e', 'MYSQL_ROOT_PASSWORD=' + state['db_password'], '-v', str(BASE / 'mysql-data') + ':/var/lib/mysql', '-v', '/opt/apimeter-server/backups:/backups:ro'], ['--character-set-server=utf8mb4', '--collation-server=utf8mb4_unicode_ci', '--max-connections=120', '--innodb-buffer-pool-size=512M', '--local-infile=0'])
    for _ in range(90):
        try:
            sql('SELECT 1', 'mysql')
            break
        except subprocess.CalledProcessError:
            time.sleep(1)
    else:
        raise SystemExit('Isolated MySQL did not become ready')
    sql('CREATE DATABASE modelsell CHARACTER SET utf8mb4; CREATE DATABASE `modelsell-log` CHARACTER SET utf8mb4;', 'mysql')
    for name in ['modelsell', 'modelsell-log']:
        begin = time.monotonic()
        run(['docker', 'exec', PREFIX + '-db', 'sh', '-c', 'exec mysql -uroot -p"$MYSQL_ROOT_PASSWORD" "$1" < "/backups/$1.sql"', 'sh', name])
        print('Imported', name, round(time.monotonic() - begin, 2), flush=True)
    # Only the freshly-created test DB is modified. Keep historical logs for realistic table size.
    sql("DELETE FROM options; UPDATE channels SET status=2, `key`='isolated-test-disabled', base_url='http://mock:8080'; UPDATE abilities SET enabled=0; UPDATE tokens SET status=2; UPDATE users SET status=2; DELETE FROM tasks; DELETE FROM midjourneys;")
    sql("INSERT INTO options (`key`,value) VALUES ('RegisterEnabled','true'),('PasswordRegisterEnabled','true'),('EmailVerificationEnabled','false'),('TurnstileCheckEnabled','false'),('PasswordLoginEnabled','true'),('QuotaForNewUser','100000000'),('DefaultGroup','default');")
    start('redis', state['images']['redis:8-alpine']['id'], [], ['redis-server', '--save', '', '--appendonly', 'no'])
    start('mock', state['images']['python:3.12-alpine']['id'], ['-v', str(BASE / 'mock.py') + ':/mock.py:ro'], ['python', '-u', '/mock.py'])
    common = {'SQL_DSN': f"root:{state['db_password']}@tcp(db:3306)/modelsell?charset=utf8mb4&parseTime=true&loc=Local", 'LOG_SQL_DSN': f"root:{state['db_password']}@tcp(db:3306)/modelsell-log?charset=utf8mb4&parseTime=true&loc=Local", 'REDIS_CONN_STRING': 'redis://redis:6379/0', 'SESSION_SECRET': state['session_secret'], 'CRYPTO_SECRET': state['session_secret'], 'GIN_MODE': 'release', 'TZ': 'Asia/Shanghai', 'MEMORY_CACHE_ENABLED': 'true', 'SYNC_FREQUENCY': '5', 'BATCH_UPDATE_ENABLED': 'true', 'BATCH_UPDATE_INTERVAL': '5', 'GLOBAL_API_RATE_LIMIT_ENABLE': 'false', 'GLOBAL_WEB_RATE_LIMIT_ENABLE': 'false', 'GLOBAL_API_RATE_LIMIT': '100000', 'GLOBAL_WEB_RATE_LIMIT': '100000', 'SHUTDOWN_TIMEOUT_SECONDS': '150', 'SESSION_COOKIE_SECURE': 'false', 'SQL_MAX_OPEN_CONNS': '30', 'ERROR_LOG_ENABLED': 'true', 'API_TEMP_IMAGE_STORAGE': 'local', 'API_TEMP_IMAGE_PUBLIC_BASE_URL': 'http://new:34102', 'API_TEMP_IMAGE_DIR': '/data/images', 'STREAMING_TIMEOUT': '150'}
    state['common_env'] = common
    (BASE / 'state.json').write_text(json.dumps(state, indent=2))
    for name, role, port, tag in [('old', 'slave', 34101, images[0]), ('new', 'master', 34102, images[1])]:
        env = {**common, 'NODE_TYPE': role, 'NODE_NAME': 'isolated-compat-' + name, 'PORT': str(port)}
        env_path = BASE / (name + '.env')
        env_path.write_text('\n'.join(k + '=' + v for k, v in env.items()) + '\n')
        os.chmod(env_path, 0o600)
        args = ['docker', 'create', '--name', PREFIX + '-' + name, '--label', 'purpose=apimeter-compat-20260913', '--network', PREFIX, '--network-alias', name, '--memory', '2g', '--cpus', '2', '--stop-timeout', '160', '--env-file', str(env_path), '-p', f'127.0.0.1:{port}:{port}', '-v', str(BASE / (name + '-data')) + ':/data', state['images'][tag]['id']]
        run(args)
    run(['docker', 'start', PREFIX + '-old'])
    print('SETUP COMPLETE: isolated old slave on 127.0.0.1:34101; new master remains stopped', flush=True)

if __name__ == '__main__':
    main()
