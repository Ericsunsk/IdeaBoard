/**
 * @deprecated P1b compatibility shim for the `shared/config` → `core/config` move (§5.5).
 * TTL: 1 phase — delete after P1 (retirement is a later package, not this one).
 * Deletion owner: p1b-core. Do not import this module in new code; use `core/config` instead.
 */
export * from '../../core/config/timing'
