/**
 * Canvas board feature public entrypoint (WP-4).
 *
 * Converges the domain model, catalog, and persistence seams. Named exports
 * stay stable so cross-feature callers are unaffected by internal renames.
 */
export * from './domain'
export * from './catalog'
export * from './persistence'
