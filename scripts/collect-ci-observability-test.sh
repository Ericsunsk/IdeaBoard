#!/usr/bin/env bash
set -euo pipefail
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
fixture="$(mktemp -d)"
trap 'rm -rf "$fixture"' EXIT
python3 - "$root/scripts/collect-ci-observability.sh" "$fixture" <<'PY'
import json
import os
from pathlib import Path
import subprocess
import sys

collector, fixture = sys.argv[1], Path(sys.argv[2])
bin_dir = fixture / 'bin'
bin_dir.mkdir()
repository = 'Ericsunsk/IdeaBoard'
base = f'/repos/{repository}/actions'
current_path = f'{base}/runs/42'
current_jobs = f'{current_path}/jobs?filter=latest&per_page=100'
history_path = f'{base}/workflows/7/runs?per_page=10&status=completed'
previous_path = f'{base}/runs/41'
previous_jobs = f'{previous_path}/jobs?filter=latest&per_page=100'
start = '2026-11-02T00:00:00Z'

def run(run_id, **changes):
    return dict({'id': run_id, 'workflow_id': 7, 'run_attempt': 2,
                 'head_repository': {'full_name': repository}, 'head_sha': 'a' * 40,
                 'path': '.github/workflows/ci.yml', 'event': 'push', 'head_branch': 'main',
                 'status': 'completed', 'conclusion': 'success',
                 'run_started_at': start, 'updated_at': '2026-11-02T00:03:00Z'}, **changes)

def job(job_id, name, end, **changes):
    return dict({'id': job_id, 'name': name, 'run_attempt': 2,
                 'status': 'completed', 'conclusion': 'success',
                 'started_at': start, 'completed_at': end}, **changes)

first = job(1, 'Ops', '2026-11-02T00:01:00Z')
second = job(2, 'API', '2026-11-02T00:02:00Z')
reporter = job(3, 'Timing evidence', None, status='in_progress', conclusion=None)
skipped = {'id': 4, 'name': 'Web', 'run_attempt': 2,
           'status': 'completed', 'conclusion': 'skipped'}
previous = run(41)
responses = {
    current_path: run(42, status='in_progress', conclusion=None),
    current_jobs: [{'jobs': [first, reporter]}, {'jobs': [first, second, skipped]}],
    history_path: {'workflow_runs': [run(42), previous,
        run(40, head_repository={'full_name': 'untrusted/fork'}),
        run(39, event='pull_request')]},
    previous_path: previous,
    previous_jobs: [{'jobs': [job(10, 'Ops', '2026-11-02T00:01:30Z')]}, {'jobs': []}],
}
(fixture / 'responses.json').write_text(json.dumps(responses))
# The collector uses the real Python summarizer; only the gh network boundary is stubbed.
(bin_dir / 'gh').write_text('''#!/usr/bin/env python3
import json
import os
from pathlib import Path
import sys
root = Path(os.environ['CI_OBS_FIXTURE'])
args = sys.argv[1:]
with (root / 'requests.jsonl').open('a') as log:
    log.write(json.dumps(args) + '\\n')
if not args or args[0] != 'api':
    sys.exit('unexpected gh subcommand')
path = args[-1]
flags = args[1:-1]
expected = ['--paginate', '--slurp'] if '/jobs?' in path else []
if flags != expected:
    sys.exit('unexpected gh pagination flags')
if path == os.environ.get('CI_OBS_FAIL_PATH'):
    sys.stderr.write('fixture API failure\\n')
    sys.exit(37)
responses = json.loads((root / 'responses.json').read_text())
if path not in responses:
    sys.exit('unexpected gh endpoint: ' + path)
print(json.dumps(responses[path]))
''')
(bin_dir / 'gh').chmod(0o755)
base_env = dict(os.environ, PATH=str(bin_dir) + os.pathsep + os.environ['PATH'],
                CI_OBS_FIXTURE=str(fixture), GITHUB_REPOSITORY=repository, GITHUB_RUN_ID='42')

def collect(name, fail_path=None, **changes):
    output = fixture / name
    summary = fixture / (name + '.md')
    summary.write_text('Existing job summary\n')
    env = dict(base_env, GITHUB_STEP_SUMMARY=str(summary), **changes)
    if fail_path:
        env['CI_OBS_FAIL_PATH'] = fail_path
    else:
        env.pop('CI_OBS_FAIL_PATH', None)
    (fixture / 'requests.jsonl').write_text('')
    completed = subprocess.run(['bash', collector, str(output)], env=env, capture_output=True, text=True)
    requests = [json.loads(line) for line in (fixture / 'requests.jsonl').read_text().splitlines()]
    return completed, output, summary, requests

completed, output, summary, requests = collect('success')
assert completed.returncode == 0, completed.stderr
assert [request[-1] for request in requests] == [current_path, current_jobs, history_path, previous_path, previous_jobs]
assert json.loads((output / 'jobs.json').read_text()) == responses[current_jobs]
history = json.loads((output / 'history.json').read_text())
assert len(history) == 1 and history[0]['run']['id'] == 41
assert history[0]['jobs'] == responses[previous_jobs]
report = json.loads((output / 'metrics.json').read_text())
assert [observation['run_id'] for observation in report['runs']] == [42, 41]
current = report['runs'][0]
assert current['repository'] == repository and current['head_sha'] == 'a' * 40
assert current['runner_seconds'] == 180 and current['runner_minutes'] == 3
assert not current['elapsed_complete'] and not current['runner_complete']
assert current['unknown_job_duration_ids'] == [3] and len(current['jobs']) == 4
stats = report['aggregate']['.github/workflows/ci.yml']
assert stats['runs'] == 2
assert stats['elapsed_seconds'] == {'samples': 1, 'median': 180.0, 'p95': 180.0}
assert stats['runner_minutes'] == {'samples': 1, 'median': 1.5, 'p95': 1.5}
assert summary.read_text() == 'Existing job summary\n' + (output / 'summary.md').read_text()
assert 'not billed cost' in summary.read_text()
# Every direct gh API boundary must propagate errors, including inside the history loop.
for index, endpoint in enumerate([current_path, current_jobs, history_path, previous_path, previous_jobs]):
    completed, output, summary, requests = collect(f'api-error-{index}', endpoint)
    assert completed.returncode == 37, (endpoint, completed.returncode, completed.stderr)
    assert requests[-1][-1] == endpoint
    assert not (output / 'metrics.json').exists()
    assert summary.read_text() == 'Existing job summary\n'
# Invalid identity never reaches gh or writes an evidence artifact.
for index, changes in enumerate([{'GITHUB_REPOSITORY': 'invalid repository/name'}, {'GITHUB_RUN_ID': '0'}]):
    completed, output, summary, requests = collect(f'bad-identity-{index}', **changes)
    assert completed.returncode != 0 and not requests and not output.exists(), (changes, completed.returncode, requests, output.exists(), completed.stderr)
print('CI observability collector fixtures passed')
PY
