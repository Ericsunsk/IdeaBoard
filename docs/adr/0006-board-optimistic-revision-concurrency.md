---
status: accepted
owner: Board
decision_date: 2026-09-11
supersedes: none
---

# Board saves use optimistic revision concurrency

Board writes must include the revision observed by the client as `base_revision`, and a successful write returns the persisted `new_revision`. The backend compares and advances the revision atomically with the update, returning HTTP 409 when the persisted revision has changed so the client can reconcile instead of silently applying last-write-wins. New Boards use base revision `0`; existing callers must hydrate or retain the latest server revision before saving.

This intentionally extends the protected Board API contract. Persisted Board schema, serialized state, media references, storage keys, and public routes remain unchanged. Review this decision if Board writes move away from PostgreSQL compare-and-swap semantics or if offline multi-writer reconciliation needs a richer conflict model; any replacement must supersede this ADR explicitly.
