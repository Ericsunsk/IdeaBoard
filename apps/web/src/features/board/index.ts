/**
 * Board feature public entrypoint.
 *
 * `board` is the only Board repository/synchronization fact source.
 * Cross-feature consumers import Board facts through this barrel;
 * Canvas `board-session` orchestrates through the narrow `ports/canvas` entry.
 *
 * Export groups are intentionally classified by responsibility:
 * - core: durable Board document facts and pure validation rules
 * - session: catalog, sync, and share-join use cases/ports
 * - session/persistence + session/preview: stable adapters with contracts
 * - ui/gallery: catalog view composition consumed by route wiring
 *
 * The generated API adapter remains private to Board session. Consumers
 * must not import `session/adapters` or any other Board implementation path.
 */
// Durable Board facts and pure domain rules.
export * from './core'
// Session use cases and repository/sync ports.
export * from './session/catalog'
export * from './session/sync'
export * from './session/share-join'
// Stable adapters for persistence and preview boundaries.
export * from './session/persistence'
export * from './session/preview'
// Catalog presentation composition (route-level public surface).
export * from './ui/gallery'
