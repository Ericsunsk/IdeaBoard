#!/usr/bin/env python3
"""Summarize native Actions timing evidence without treating it as a billing API."""
import argparse
import datetime as dt
import json
import math
from pathlib import Path
import statistics


def timestamp(value):
    if not value:
        return None
    try:
        result = dt.datetime.fromisoformat(value.replace("Z", "+00:00"))
        return result if result.tzinfo else None
    except (TypeError, ValueError):
        return None


def seconds(start, end):
    a, b = timestamp(start), timestamp(end)
    return round((b - a).total_seconds(), 3) if a and b and b >= a else None


def pages(value, key):
    if isinstance(value, dict):
        return value.get(key, [])
    if isinstance(value, list):
        return [row for page in value for row in (page.get(key, []) if isinstance(page, dict) and key in page else [page])]
    raise ValueError("expected JSON object or array")


def summarize(run, jobs, observed_at):
    # gh jobs?filter=latest selects an attempt; ids also prevent pagination overlap.
    unique = {job["id"]: job for job in pages(jobs, "jobs") if isinstance(job, dict) and "id" in job}
    attempt = run.get("run_attempt")
    selected = [job for job in unique.values() if not attempt or job.get("run_attempt", attempt) == attempt]
    rows, first_failure = [], []
    started = run.get("run_started_at") or run.get("created_at")
    missing = []
    for job in sorted(selected, key=lambda item: (item.get("name", ""), item["id"])):
        skipped = job.get("conclusion") == "skipped"
        duration = 0 if skipped else seconds(job.get("started_at"), job.get("completed_at"))
        if duration is None:
            missing.append(job["id"])
        rows.append({"id": job["id"], "name": job.get("name"), "status": job.get("status"),
                     "conclusion": job.get("conclusion"), "duration_seconds": duration,
                     "started_at": job.get("started_at"), "completed_at": job.get("completed_at")})
        for step in job.get("steps", []):
            if step.get("conclusion") in ("failure", "timed_out", "startup_failure"):
                latency = seconds(started, step.get("completed_at"))
                if latency is not None:
                    first_failure.append(latency)
    completed = run.get("status") == "completed"
    # A final metrics job observes its own run before GitHub has marked it complete.
    end = run.get("updated_at") if completed else observed_at
    duration = seconds(started, end)
    known = [row["duration_seconds"] for row in rows if row["duration_seconds"] is not None]
    runner_seconds = round(sum(known), 3)
    return {"run_id": run.get("id"), "run_attempt": attempt, "workflow": run.get("path"),
            "repository": (run.get("repository") or run.get("head_repository") or {}).get("full_name"),
            "head_sha": run.get("head_sha"), "event": run.get("event"), "branch": run.get("head_branch"),
            "status": run.get("status"), "conclusion": run.get("conclusion"),
            "html_url": run.get("html_url"), "observed_at": observed_at,
            "elapsed_seconds": duration, "elapsed_complete": completed and duration is not None,
            "runner_seconds": runner_seconds, "runner_minutes": round(runner_seconds / 60, 3),
            "runner_complete": not missing, "unknown_job_duration_ids": missing,
            "first_actionable_failure_seconds": min(first_failure) if first_failure else None,
            "jobs": rows}


def distribution(values):
    if not values:
        return {"samples": 0, "median": None, "p95": None}
    ordered = sorted(values)
    return {"samples": len(ordered), "median": statistics.median(ordered),
            "p95": ordered[max(0, math.ceil(len(ordered) * .95) - 1)]}


def aggregate(runs):
    groups = {}
    # Avoid one run's rerun or duplicate pagination entry weighting history twice.
    latest = {}
    for run in runs:
        key = (run.get("repository"), run.get("run_id"))
        if key not in latest or (run.get("run_attempt") or 0) > (latest[key].get("run_attempt") or 0):
            latest[key] = run
    for run in latest.values():
        group = groups.setdefault(run.get("workflow") or "unknown", [])
        group.append(run)
    return {name: {"runs": len(group),
                   "elapsed_seconds": distribution([r["elapsed_seconds"] for r in group if r["elapsed_complete"]]),
                   "runner_minutes": distribution([r["runner_minutes"] for r in group if r["runner_complete"]]),
                   "first_actionable_failure_seconds": distribution([r["first_actionable_failure_seconds"] for r in group if r["first_actionable_failure_seconds"] is not None])}
            for name, group in sorted(groups.items())}


def markdown(report):
    def cell(value):
        return str(value if value is not None else "unknown").replace("|", "\\|").replace("\n", " ").replace("\r", " ")
    lines = ["### CI/CD timing evidence", "", "Native Actions timestamps; runner duration is an estimate of allocated job time, not billed cost.",
             "Incomplete jobs are excluded from the known runner total and listed in JSON. A reporting job may observe its own run before completion.", "",
             "| Run | SHA | Elapsed seconds | Known runner minutes | First failure seconds |", "| --- | --- | ---: | ---: | ---: |"]
    for run in report["runs"]:
        lines.append("| " + " | ".join(cell(v) for v in (run["run_id"], (run["head_sha"] or "")[:12],
                     run["elapsed_seconds"], run["runner_minutes"], run["first_actionable_failure_seconds"])) + " |")
    lines += ["", "History median/p95 uses complete observations only; zero samples means unavailable evidence.", "",
              "| Workflow | Runs | Elapsed median / p95 seconds | Runner median / p95 minutes |", "| --- | ---: | ---: | ---: |"]
    for workflow, group in report["aggregate"].items():
        elapsed, runner = group["elapsed_seconds"], group["runner_minutes"]
        lines.append(f"| {cell(workflow)} | {group['runs']} | {cell(elapsed['median'])} / {cell(elapsed['p95'])} | {cell(runner['median'])} / {cell(runner['p95'])} |")
    return "\n".join(lines) + "\n"


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--runs", type=Path, required=True)
    parser.add_argument("--jobs", type=Path, required=True)
    parser.add_argument("--history", type=Path, help="JSON array of {run, jobs} observations")
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--summary", type=Path, required=True)
    parser.add_argument("--observed-at", default=dt.datetime.now(dt.timezone.utc).isoformat())
    args = parser.parse_args()
    raw = json.loads(args.runs.read_text())
    run_rows = [raw] if isinstance(raw, dict) and "id" in raw else pages(raw, "workflow_runs")
    if len(run_rows) != 1:
        parser.error("--runs must identify exactly one run; supply history as {run,jobs} pairs")
    observations = [summarize(run_rows[0], json.loads(args.jobs.read_text()), args.observed_at)]
    if args.history:
        for pair in json.loads(args.history.read_text()):
            observations.append(summarize(pair["run"], pair["jobs"], args.observed_at))
    report = {"schema_version": 1, "measurement": "native_actions_timestamps_not_billing", "runs": observations,
              "aggregate": aggregate(observations)}
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.summary.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(report, indent=2) + "\n")
    args.summary.write_text(markdown(report))


if __name__ == "__main__":
    main()
