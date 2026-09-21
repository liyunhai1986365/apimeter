import collections
import difflib
import json
import re
import subprocess
import client as c

BASE = c.BASE
result = json.loads((BASE / 'result.json').read_text())
old_protected = json.loads((BASE / 'protected-before.json').read_text())
final = {}

def schema_tables(text):
    return {m.group(1): re.sub(r' AUTO_INCREMENT=\d+', '', m.group(0)) for m in re.finditer(r'CREATE TABLE `([^`]+)` \([\s\S]*?\) ENGINE=[^;]+;', text)}

schema_summary = {}
diffs = []
for db in ['modelsell', 'modelsell-log']:
    before = schema_tables((BASE / ('before-' + db + '-schema.sql')).read_text())
    after = schema_tables((BASE / ('after-' + db + '-schema.sql')).read_text())
    added = sorted(set(after) - set(before))
    removed = sorted(set(before) - set(after))
    changed = sorted(k for k in before.keys() & after.keys() if before[k] != after[k])
    schema_summary[db] = {'before_tables': len(before), 'after_tables': len(after), 'added': added, 'removed': removed, 'changed': changed}
    for table in sorted(set(added + removed + changed)):
        diffs.extend(difflib.unified_diff(before.get(table, '').splitlines(), after.get(table, '').splitlines(), fromfile=db + '/' + table + ':before', tofile=db + '/' + table + ':after', lineterm=''))
(BASE / 'schema-diff.txt').write_text('\n'.join(diffs) + '\n')
final['schema'] = schema_summary
uid = c.state['users']['compatuser']
start_id = int(result['before']['log_max_id'])
end_id = int(result['after']['log_max_id'])
where = f'user_id={uid} AND id>{start_id} AND id<={end_id} AND type=2'
final['billing_window'] = {'rows_sum_min_max': c.sql('SELECT COUNT(*),SUM(quota),MIN(quota),MAX(quota) FROM logs WHERE ' + where, 'modelsell-log').strip(), 'duplicate_request_ids': c.sql('SELECT COUNT(*) FROM (SELECT request_id FROM logs WHERE ' + where + " AND request_id<>'' GROUP BY request_id HAVING COUNT(*)>1) duplicates", 'modelsell-log').strip()}
log_summary = {}
for node in ['old', 'new']:
    p = subprocess.run(['docker', 'logs', '--timestamps', c.PREFIX + '-' + node], stdout=subprocess.PIPE, stderr=subprocess.STDOUT, check=True)
    (BASE / (node + '-app.log')).write_bytes(p.stdout)
    text = p.stdout.decode(errors='replace')
    log_summary[node] = {name: len(re.findall(pattern, text, re.I)) for name, pattern in {'unknown_column': r'unknown column', 'missing_table': r"table .{0,150}doesn.t exist", 'deadlock': r'deadlock found', 'lock_timeout': r'lock wait timeout exceeded', 'panic': r'panic recovered|panic detected|^panic:', 'database_migration_started': r'database migration started'}.items()}
final['log_checks'] = log_summary
network = json.loads(c.command(['docker', 'network', 'inspect', c.PREFIX]))[0]
final['isolated_network'] = {'name': network['Name'], 'internal': network['Internal'], 'containers': sorted(v['Name'] for v in network['Containers'].values())}
final['test_container_ports'] = {}
for node in ['old', 'new', 'mock', 'redis', 'db']:
    name = c.PREFIX + '-' + node
    d = json.loads(c.command(['docker', 'inspect', name]))[0]
    if d['Config'].get('Labels', {}).get('purpose') != 'apimeter-compat-20260913':
        raise SystemExit('Unexpected container ownership: ' + name)
    final['test_container_ports'][name] = d['NetworkSettings']['Ports']
# Stop only the five containers created by this test, preserving data and evidence.
c.command(['docker', 'stop', c.PREFIX + '-old', c.PREFIX + '-new'])
c.command(['docker', 'stop', c.PREFIX + '-mock', c.PREFIX + '-redis', c.PREFIX + '-db'])
originals = json.loads(c.command(['docker', 'inspect', *old_protected.keys()]))
now = {r['Name']: {'id': r['Id'], 'started_at': r['State']['StartedAt'], 'status': r['State']['Status'], 'restarts': r['RestartCount']} for r in originals}
final['existing_containers_unchanged'] = now == old_protected
final['existing_containers'] = now
tests = json.loads(c.command(['docker', 'inspect', *[c.PREFIX + '-' + n for n in ['old', 'new', 'mock', 'redis', 'db']]]))
final['test_containers_stopped'] = all(r['State']['Status'] == 'exited' for r in tests)
(BASE / 'final-audit.json').write_text(json.dumps(final, ensure_ascii=False, indent=2))
print(json.dumps({k:v for k,v in final.items() if k != 'existing_containers'}, ensure_ascii=False, indent=2), flush=True)
