/**
 * @deprecated P1b compatibility shim for the `shared/components` → `core/components` move (§5.5).
 * TTL: 1 phase — delete after P1 (retirement is a later package, not this one).
 * Deletion owner: p1b-core. Do not import this module in new code; use `core/components/StatusBadge.vue` instead.
 */
export { default } from '../../core/components/StatusBadge.vue'
export type { AppStatusTone, AppStatusBadgeSize } from '../../core/components/StatusBadge.vue'
