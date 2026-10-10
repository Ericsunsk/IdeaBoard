/**
 * @deprecated P1b compatibility shim for the `shared/interaction` → `core/interaction` move (§5.5).
 * TTL: 1 phase — delete after P1 (retirement is a later package, not this one).
 * Deletion owner: p1b-core. Do not import this module in new code; use `core/interaction` instead.
 */
export * from '../../core/interaction/interactiveControlState'
