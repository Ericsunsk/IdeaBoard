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
export { useWorkspaceSwitcher } from './shell/useWorkspaceSwitcher'
export { default as WorkspaceNavigation } from './shell/components/WorkspaceNavigation.vue'
export { default as WorkspaceHeader } from './shell/components/WorkspaceHeader.vue'
export {
  classifyWorkspaceAccessError,
  deleteWorkspace,
  effectiveOrganizationWorkspaceAccess,
  fetchAllWorkspaces,
  fetchOrganizationMembers,
  fetchOrganizations,
  fetchPrincipal,
  fetchWorkspacePage,
  inviteOrganizationMember,
  normalizeWorkspaceListParams,
  parseOrganizationMember,
  parsePrincipal,
  parseWorkspaceItem,
  parseWorkspacePage,
  provisionOrganization,
  provisionWorkspace,
  removeOrganizationMember,
  renameWorkspace,
  updateOrganizationMember,
  workspaceApiUrls,
  workspaceQueryKeys,
} from './selection/workspaceResource'
export type {
  OrganizationMemberSummary,
  OrganizationRole,
  OrganizationSummary,
  PrincipalSummary,
  WorkspaceAccessKind,
  WorkspaceListPage,
  WorkspaceSummary,
} from './selection/workspaceResource'
export {
  createOrganizationScopeStore,
  getOrganizationScopeStore,
  installOrganizationScopeStore,
  localStorageOrganizationScopePersist,
} from './organization/scope'
export type { OrganizationScope, OrganizationScopeStore } from './organization/scope'
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
  fetchWorkspaceMembers,
  workspaceMembersQueryOptions,
} from './selection/workspaceMembersResource'
export type { WorkspaceMemberEntry } from './selection/workspaceMembersResource'
