/**
 * Canvas board catalog public entrypoint (WP-4).
 *
 * External consumers import catalog facts and commands through this barrel.
 * Internal modules stay on relative imports.
 */
export * from './boards'
export * from './catalog'
export * from './commands'
export * from './boardSync'
export * from './joinSharedBoardWorkflow'
