/**
 * Board feature public entrypoint.
 *
 * `board` is the only Board repository/synchronization fact source.
 * Cross-feature consumers import Board facts through this barrel;
 * Canvas `board-session` orchestrates on top of it.
 */
export * from './domain'
export * from './application/catalog'
export * from './application/sync'
export * from './application/collaboration'
export * from './infrastructure/persistence'
export * from './infrastructure/preview'
export * from './presentation/catalog'
