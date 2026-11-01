#!/usr/bin/env bash
set -euo pipefail
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
fixture="$(mktemp -d)"
trap 'rm -rf "$fixture"' EXIT
python3 - "$root/scripts/ci-observability.py" "$fixture" <<'PY'
import json
from pathlib import Path
import subprocess
import sys
script, destination = sys.argv[1], Path(sys.argv[2])
run = {'id': 123, 'run_attempt': 2, 'head_sha': 'a'*40, 'path': '.github/workflows/ci.yml',
       'head_repository': {'full_name':'Ericsunsk/IdeaBoard'}, 'event':'push','head_branch':'main',
       'status':'completed','conclusion':'failure','run_started_at':'2026-11-02T00:00:00Z','updated_at':'2026-11-02T00:02:00Z'}
a = {'id':1,'name':'API','run_attempt':2,'status':'completed','conclusion':'success','started_at':'2026-11-02T00:00:00Z','completed_at':'2026-11-02T00:01:00Z'}
b = {'id':2,'name':'Web','run_attempt':2,'status':'completed','conclusion':'failure','started_at':'2026-11-02T00:00:00Z','completed_at':'2026-11-02T00:02:00Z',
     'steps':[{'name':'Unit','conclusion':'failure','completed_at':'2026-11-02T00:01:30Z'}]}
skipped = {'id':3,'name':'Auth','run_attempt':2,'status':'completed','conclusion':'skipped'}
old = dict(a,id=4,run_attempt=1,completed_at='2026-11-02T01:00:00Z')
def execute(observation, jobs, history=None):
    (destination/'run.json').write_text(json.dumps(observation)); (destination/'jobs.json').write_text(json.dumps(jobs))
    command=['python3',script,'--runs',str(destination/'run.json'),'--jobs',str(destination/'jobs.json'),
             '--output',str(destination/'metrics.json'),'--summary',str(destination/'summary.md'),'--observed-at','2026-11-02T00:03:00Z']
    if history is not None:
        (destination/'history.json').write_text(json.dumps(history));command+=['--history',str(destination/'history.json')]
    subprocess.run(command,check=True,capture_output=True)
    return json.loads((destination/'metrics.json').read_text())
report=execute(run,[{'jobs':[a,b,skipped]},{'jobs':[a,old]}])
r=report['runs'][0]
assert r['elapsed_seconds']==120 and r['runner_seconds']==180 and r['runner_minutes']==3
assert r['first_actionable_failure_seconds']==90 and len(r['jobs'])==3 and r['runner_complete']
assert r['head_sha']=='a'*40 and r['repository']=='Ericsunsk/IdeaBoard'
assert 'not billed cost' in (destination/'summary.md').read_text()
partial=dict(run,status='in_progress',conclusion=None)
unknown={'id':8,'name':'Metrics','run_attempt':2,'status':'in_progress','started_at':'2026-11-02T00:02:00Z'}
r=execute(partial,{'jobs':[a,unknown]})['runs'][0]
assert r['elapsed_seconds']==180 and not r['elapsed_complete'] and not r['runner_complete']
assert r['runner_seconds']==60 and r['unknown_job_duration_ids']==[8]
invalid=dict(a,completed_at='2026-11-01T00:00:00Z')
r=execute(run,{'jobs':[invalid]})['runs'][0]
assert r['runner_seconds']==0 and not r['runner_complete'] and r['unknown_job_duration_ids']==[1]
# History statistics use each latest run once and never manufacture missing times.
other=dict(run,id=124,updated_at='2026-11-02T00:04:00Z')
history=[{'run':other,'jobs':{'jobs':[a]}},{'run':dict(run,run_attempt=1),'jobs':{'jobs':[old]}}]
report=execute(run,{'jobs':[a,b]},history)
stats=report['aggregate']['.github/workflows/ci.yml']
assert stats['runs']==2 and stats['elapsed_seconds']=={'samples':2,'median':180.0,'p95':240.0}
assert stats['runner_minutes']=={'samples':2,'median':2.0,'p95':3.0}
# Refuse ambiguous current run; one job set must never be assigned to multiple runs.
(destination/'run.json').write_text(json.dumps([{'workflow_runs':[run,other]}]))
failed=subprocess.run(['python3',script,'--runs',str(destination/'run.json'),'--jobs',str(destination/'jobs.json'),
 '--output',str(destination/'bad.json'),'--summary',str(destination/'bad.md')],capture_output=True)
assert failed.returncode!=0 and not (destination/'bad.json').exists()
print('CI observability timing fixtures passed')
PY
