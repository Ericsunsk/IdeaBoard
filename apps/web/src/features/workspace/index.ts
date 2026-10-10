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
} from './selection/workspaceResource'
export type {
  PrincipalSummary,
  WorkspaceAccessKind,
  WorkspaceListPage,
  WorkspaceSummary,
} from './selection/workspaceResource'
export {
  createWorkspaceSelectionStore,
  getWorkspaceSelectionStore,
  installWorkspaceSelectionStore,
  localStorageWorkspacePersist,
  memoryWorkspacePersist,
  resetWorkspaceSelectionStore,
  resolveWorkspaceSelection,
  subscribeWorkspaceSelectionChange,
  toWorkspaceSelectionStatus,
  useWorkspaceSelection,
} from './selection/workspaceContext'
export type {
  ProvisionWorkspaceInput,
  WorkspaceProvisionFn,
  WorkspaceSelectionPersist,
  WorkspaceSelectionSource,
  WorkspaceSelectionStatus,
  WorkspaceSelectionStore,
} from './selection/workspaceContext'
export {
  getWorkspaceSelectionPort,
  installWorkspaceSelectionPort,
  memoryWorkspaceSelectionPersist,
  resetWorkspaceSelectionPort,
  useWorkspaceSelectionPort,
} from './selection/workspaceSelectionPort'
export type { WorkspaceSelectionPort } from './selection/workspaceSelectionPort'
