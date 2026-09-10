/**
 * Workspace feature public entrypoint.
 *
 * `workspace` owns workspace shell components and navigation controls.
 * Workspace routes are loaded by the application router via
 * `workspace/routes/workspaceRoutes` to avoid route-level circular dependencies
 * and preserve dynamic chunk splitting.
 */
export { useWorkspaceAccountControls } from './shell/useWorkspaceAccountControls'
export type { WorkspaceNavigationAdapter } from './shell/useWorkspaceAccountControls'
export { default as WorkspaceNavigation } from './shell/components/WorkspaceNavigation.vue'
