import json
import time
import client as c

results = {}

def check(label, *args, **kwargs):
    response = c.request(*args, **kwargs)
    results[label] = response[0]
    print(label, json.dumps(response[0], ensure_ascii=False), flush=True)
    return response

def main():
    c.command(['docker', 'start', c.PREFIX + '-new'])
    for _ in range(60):
        if c.request('new', '/api/ready')[0]['ok']:
            break
        time.sleep(1)
    protocol = json.dumps({'protocol': {'native_modes': ['openai.chat', 'openai.image.generations']}})
    c.sql("UPDATE channels SET setting='" + protocol + "' WHERE id=" + str(c.state['channel_id']))
    time.sleep(6)
    key = {'Authorization': 'Bearer sk-' + c.state['tokens']['compatrevoke']['key']}
    for submit_node in ['old', 'new']:
        response = check(submit_node + '_async_image_submit', submit_node, '/v1/images/generations?async=true', 'POST', {'model': 'dall-e-3', 'prompt': 'isolated task compatibility', 'n': 1, 'size': '1024x1024', 'response_format': 'b64_json'}, key, timeout=20)
        body = response[1] or {}
        task_id = body.get('id')
        results[submit_node + '_async_shape'] = {'keys': list(body), 'task_id_present': bool(task_id)}
        if task_id:
            for node in ['old', 'new']:
                check(submit_node + '_task_initial_on_' + node, node, '/v1/tasks/' + task_id, headers=key)
            time.sleep(12)
            for node in ['old', 'new']:
                response = check(submit_node + '_task_final_on_' + node, node, '/v1/tasks/' + task_id, headers=key)
                body = response[1] or {}
                results[submit_node + '_task_state_on_' + node] = {'state': body.get('state') or body.get('status'), 'has_data': bool(body.get('data')), 'error': body.get('error')}
    new_session = c.login('new', 'compatrevoke')
    if new_session:
        check('new_session_before_logout_on_old', 'old', '/api/user/self', headers=new_session)
        check('new_logout', 'new', '/api/user/logout', headers=new_session)
        check('revoked_new_session_on_new', 'new', '/api/user/self', headers=new_session)
        check('revoked_new_session_on_old', 'old', '/api/user/self', headers=new_session)
    username = 'compatpost'
    if not c.sql("SELECT id FROM users WHERE username='compatpost'").strip():
        check('old_register_after_migration', 'old', '/api/user/register', 'POST', {'username': username, 'password': c.state['password']})
    uid = c.sql("SELECT id FROM users WHERE username='compatpost'").strip()
    if uid:
        c.state['users'][username] = int(uid)
        c.save_state()
        results['old_created_user_auth_version'] = c.sql('SELECT auth_version FROM users WHERE id=' + uid).strip()
        results['old_created_user_new_login'] = bool(c.login('new', username))
    uid = c.state['users']['compatrevoke']
    results['image_tasks_db'] = c.sql(f"SELECT platform,status,quota,progress FROM tasks WHERE user_id={uid} ORDER BY id").strip()
    c.BASE.joinpath('extra-result.json').write_text(json.dumps(results, ensure_ascii=False, indent=2))
    print('EXTRA COMPLETE', flush=True)

if __name__ == '__main__':
    main()
