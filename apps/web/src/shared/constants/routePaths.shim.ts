/**
 * @deprecated P1b compatibility shim for the `shared/constants/routePaths` split (§5.5).
 * TTL: 1 phase — delete after P1 (retirement is a later package, not this one).
 * Deletion owner: p1b-core. Do not import this module in new code; use
 * `features/workspace/routePaths`, `features/admin/routePaths`,
 * or `features/legal/routePaths` instead.
 */
export * from '../../features/workspace/routePaths'
export * from '../../features/admin/routePaths'
export * from '../../features/legal/routePaths'
