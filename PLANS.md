# Workspace Task Restoration ExecPlan

## Goal

Complete the workspace task production path for issues #825 through #830:
submit, execute, render, settle, and materialize while preserving the #809
hard cut without weakening release guards.

## Scope

- #825: compile and persist a durable workflow execution plan for v1 submits.
- #826: persist task billing snapshots and settlement facts in the existing
  settlement transaction.
- #827: wire the production result fetcher and derivative renderer.
- #828: wire the library saver and make materialization billing/link state
  converge correctly on errors.
- #829: preserve the intentional #809 hard cut. No legacy routes, handlers,
  adapters, aliases, or dual-read/dual-write compatibility paths are kept.
- #830: retain the grant audit as documentation only.
- Restore the Board state contract guard removed by this diff.

## Success Criteria

- A submitted task carries a valid durable execution plan and can reach the
  provider gateway.
- Task state, Credits commands, and task billing facts commit atomically.
- Remote provider outputs become media assets and materialize into Quick Saves.
- The #809 hard cut is documented and tested.
- Targeted Go and Web checks pass with no generated-code hand edits.

## Risks And Rollback

- Transactional billing changes affect financial facts; all writes use the
  existing pgx transaction and Credits command seams.
- Remote output fetching is constrained to trusted provider origins and size
  limits. A fetch or render failure remains terminal and releases task billing.
- Each issue is committed independently, so rollback is a commit revert.
