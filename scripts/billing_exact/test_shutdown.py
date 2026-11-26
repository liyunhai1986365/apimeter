import json
import os
from pathlib import Path
import signal
import subprocess
import sys
import tempfile
import time
import unittest
from contextlib import nullcontext
from unittest.mock import patch

import local_services
import run


def run_fixture(work):
    # Exercise main's actual gateway reaping and label-scoped cleanup without Docker.
    def fake_command(argv, **kwargs):
        if argv[:2] == ['docker', 'run']:
            container = 'mysql' if 'mysql:5.7.44' in argv else 'redis'
            (work / container).write_text(argv[argv.index('--label') + 1])
            return container
        if argv[:2] == ['docker', 'port']:
            return '127.0.0.1:12345'
        if argv[:2] == ['docker', 'ps']:
            label = argv[-1].removeprefix('label=')
            return '\n'.join(c for c in ('mysql', 'redis') if (work / c).read_text() == label)
        if argv[:2] == ['docker', 'rm']:
            os.kill(os.getpid(), signal.SIGTERM)  # second signal during cleanup
            for container in argv[4:]:
                (work / container).unlink()
        return ''

    def start_gateway(suite):
        suite.app = subprocess.Popen([sys.executable, '-c', '''
import os, signal, sys
from pathlib import Path
signal.signal(signal.SIGTERM, lambda *_: sys.exit(0))
Path(sys.argv[1]).write_text(str(os.getpid()))
signal.pause()
''', str(work / 'pid')])

    def wait_for_stop(suite, *_):
        while True:
            suite.client()
            time.sleep(.01)

    sys.argv = ['run.py', '--binary', sys.executable]
    with (patch('tempfile.tempdir', str(work)),
          patch.object(local_services, '_run', fake_command),
          patch.object(local_services._Services, 'db', return_value=[]),
          patch.object(local_services._Services, 'ping_redis_from_host'),
          patch.object(run, 'serve', nullcontext), patch.object(run, 'Server'),
          patch.object(run, 'VideoServer'), patch.object(run.Suite, 'start_app', start_gateway),
          patch.object(run.Suite, 'setup', wait_for_stop)):
        run.main()


class ShutdownTest(unittest.TestCase):
    def test_sigterm_reaps_gateway_and_removes_run_containers(self):
        with tempfile.TemporaryDirectory() as tmp:
            work = Path(tmp)
            with subprocess.Popen([sys.executable, __file__, '--fixture', tmp],
                                  stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True) as runner:
                try:
                    limit = time.monotonic() + 10
                    while not (work / 'pid').exists() and runner.poll() is None and time.monotonic() < limit:
                        time.sleep(.02)
                    self.assertTrue((work / 'pid').exists(), "fixture did not start")
                    pid = int((work / 'pid').read_text())
                    runner.terminate()
                    _, errors = runner.communicate(timeout=10)
                    self.assertEqual(runner.returncode, 128 + signal.SIGTERM, errors)
                    with self.assertRaises(ProcessLookupError):
                        os.kill(pid, 0)
                    self.assertFalse((work / 'mysql').exists())
                    self.assertFalse((work / 'redis').exists())
                    checks = json.loads(next(work.glob('*/checks.json')).read_text())
                    self.assertEqual([(c['passed'], c['code']) for c in checks
                                      if c['name'].startswith('gateway-exit:')], [(True, 0)])
                finally:
                    if runner.poll() is None:
                        runner.kill()
                    if (work / 'pid').exists():
                        try:
                            os.kill(int((work / 'pid').read_text()), signal.SIGKILL)
                        except ProcessLookupError:
                            pass


if __name__ == '__main__' and sys.argv[1:2] == ['--fixture']:
    run_fixture(Path(sys.argv[2]))
