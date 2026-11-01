#!/usr/bin/env bash
# Read-only native timings; the reporter's unfinished allocation is explicitly unknown.
set -euo pipefail
: "${GITHUB_REPOSITORY:?GITHUB_REPOSITORY is required}"
: "${GITHUB_RUN_ID:?GITHUB_RUN_ID is required}"
output="${1:?usage: collect-ci-observability.sh output-directory}"
[[ "$GITHUB_REPOSITORY" =~ ^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$ && "$GITHUB_RUN_ID" =~ ^[1-9][0-9]*$ ]] || exit 1
mkdir -p "$output"
gh api "/repos/${GITHUB_REPOSITORY}/actions/runs/${GITHUB_RUN_ID}" > "$output/run.json"
gh api --paginate --slurp "/repos/${GITHUB_REPOSITORY}/actions/runs/${GITHUB_RUN_ID}/jobs?filter=latest&per_page=100" > "$output/jobs.json"
workflow_id="$(jq -er '.workflow_id | select(type == "number" and . > 0)' "$output/run.json")"
gh api "/repos/${GITHUB_REPOSITORY}/actions/workflows/${workflow_id}/runs?per_page=10&status=completed" > "$output/history-runs.json"
printf '[]\n' > "$output/history.json"
while IFS= read -r previous_id; do
  gh api "/repos/${GITHUB_REPOSITORY}/actions/runs/${previous_id}" > "$output/history-run.json"
  gh api --paginate --slurp "/repos/${GITHUB_REPOSITORY}/actions/runs/${previous_id}/jobs?filter=latest&per_page=100" > "$output/history-jobs.json"
  jq -s '.[0] + [{run:.[1],jobs:.[2]}]' "$output/history.json" "$output/history-run.json" "$output/history-jobs.json" > "$output/merged-history.json"
  mv "$output/merged-history.json" "$output/history.json"
done < <(jq -r --argjson current "$GITHUB_RUN_ID" --arg repository "$GITHUB_REPOSITORY" \
  '.workflow_runs[] | select(.id != $current and .head_repository.full_name == $repository and .event == "push") | .id' "$output/history-runs.json")
python3 "$(dirname "${BASH_SOURCE[0]}")/ci-observability.py" \
  --runs "$output/run.json" --jobs "$output/jobs.json" --history "$output/history.json" \
  --output "$output/metrics.json" --summary "$output/summary.md"
cat "$output/summary.md" >> "${GITHUB_STEP_SUMMARY:-/dev/stdout}"
