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
export { default as WorkspaceHeader } from './shell/components/WorkspaceHeader.vue'
export {
  classifyWorkspaceAccessError,
  fetchAllWorkspaces,
  fetchPrincipal,
  fetchWorkspacePage,
  normalizeWorkspaceListParams,
  parsePrincipal,
  parseWorkspaceItem,
  parseWorkspacePage,
  provisionWorkspace,
  workspaceApiUrls,
  workspaceQueryKeys,
} from './infrastructure/workspaceResource'
export type {
  PrincipalSummary,
  WorkspaceAccessKind,
  WorkspaceListPage,
  WorkspaceSummary,
} from './infrastructure/workspaceResource'
export {
  createWorkspaceSelectionStore,
  getWorkspaceSelectionStore,
  localStorageWorkspacePersist,
  memoryWorkspacePersist,
  resetWorkspaceSelectionStore,
  resolveWorkspaceSelection,
  toWorkspaceSelectionStatus,
  useWorkspaceSelection,
} from './infrastructure/workspaceContext'
export type {
  ProvisionWorkspaceInput,
  WorkspaceSelectionSource,
  WorkspaceSelectionStatus,
  WorkspaceSelectionStore,
} from './infrastructure/workspaceContext'
