/**
 * Board feature public entrypoint.
 *
 * `board` is the only Board repository/synchronization fact source.
 * Cross-feature consumers import Board facts through this barrel;
 * Canvas `board-session` orchestrates through the narrow `ports/canvas` entry.
 *
 * Export groups are intentionally classified by responsibility:
 * - domain: durable Board document facts and pure validation rules
 * - application: catalog, sync, and collaboration use cases/ports
 * - infrastructure: persistence and preview adapters with stable contracts
 * - presentation: catalog view composition consumed by route wiring
 *
 * The generated API adapter remains private to Board infrastructure. Consumers
 * must not import `infrastructure/api` or any other Board implementation path.
 */
// Durable Board facts and pure domain rules.
export * from './domain'
// Application use cases and repository/sync ports.
export * from './application/catalog'
export * from './application/sync'
export * from './application/collaboration'
// Stable adapters for persistence and preview boundaries.
export * from './infrastructure/persistence'
export * from './infrastructure/preview'
// Catalog presentation composition (route-level public surface).
export * from './presentation/catalog'
