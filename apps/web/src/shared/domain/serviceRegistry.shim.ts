/**
 * @deprecated P1b compatibility shim for the `shared/domain/serviceRegistry` split (§10.3).
 * TTL: 1 phase — delete after P1 (retirement is a later package, not this one).
 * Deletion owner: p1b-domain. Do not import this module in new code; use
 * `features/admin/prompts/core/promptVariables`, `features/canvas/core/nodes/serviceLabelKey`,
 * or `features/canvas/session/serviceImageActionText` instead.
 */
export * from '../../features/admin/prompts/core/promptVariables'
export * from '../../features/canvas/core/nodes/serviceLabelKey'
export * from '../../features/canvas/session/serviceImageActionText'
