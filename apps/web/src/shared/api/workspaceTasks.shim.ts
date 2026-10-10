/**
 * @deprecated P1b compatibility shim for the `shared/api` → `api` move (§10.4).
 * TTL: 1 phase — delete after P1 (retirement is a later package, not this one).
 * Deletion owner: p1b-api. Do not import this module in new code; use `api/workspaceTasks` instead.
 */
export * from '../../api/workspaceTasks'
