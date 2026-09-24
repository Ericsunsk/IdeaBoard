/**
 * Library feature public entrypoint (issue #679).
 *
 * Canvas, Search, and any other feature consume Library exclusively through
 * this barrel. Deep relative imports into Library internals are not allowed
 * outside this feature.
 */
export * from './workspace'
export * from './transfer'
export * from './folderFacts'
export * from './confirmations'
export * from './useLibraryWorkspace'

/** Lazily load the floating island shell to keep it out of the initial bundle. */
export const loadLibraryFloatingIsland = () => import('./components/LibraryFloatingIsland.vue')
