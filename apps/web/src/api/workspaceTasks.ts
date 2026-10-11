// Workspace task lifecycle resource (issue #805, Public Data Plane).
//
// Stable port barrel (issue #859): the contract, transport, and waiter
// seams live in `workspaceTaskContract.ts`, `workspaceTaskTransport.ts`,
// and `workspaceTaskWaiter.ts`. This module re-exports the full surface so
// existing import paths keep working; new code should import the narrow
// seam it needs (each seam is an independent test double boundary).
//
// Transport rides `publicMutator` (`/api/v1/...`) through the generated
// functions; the contract seam keeps the opaque-id asserts, status
// convergence (`cancelled_with_results` -> `cancelled`), storage-leak
// guards, and terminal waiters (SSE with polling fallback) that have no
// generated equivalent. Route pages must consume the canvas runner, never
// this module directly.
//
// Legacy `/api/user` task behavior (submit/readiness/matrix/cancel/wait) is
// retired (W2 stream 1): canvas owns its workspace task port in
// `features/canvas/session/execution/workspace/canvasWorkspaceTaskAdapter`.

export {
  assertWorkspaceTaskAssetId,
  assertWorkspaceTaskId,
  assertWorkspaceTaskWorkspaceId,
  isWorkspaceTaskTerminal,
  mapWorkspaceTaskStatus,
  toWorkspaceTask,
  toWorkspaceTaskOutputs,
  workspaceTaskLocation,
  type CreateWorkspaceTaskInput,
  type WorkspaceTaskBillingSnapshot,
  type WorkspaceTaskItem,
  type WorkspaceTaskOutputRef,
  type WorkspaceTasksRequestOptions,
  type WorkspaceTaskStatus,
} from './workspaceTaskContract'
export {
  cancelWorkspaceTask,
  createWorkspaceTask,
  getWorkspaceTask,
  isWorkspaceTaskGoneError,
  listWorkspaceTaskOutputs,
  listWorkspaceTasks,
  quoteWorkspaceTask,
} from './workspaceTaskTransport'
export { waitWorkspaceTaskTerminal, type WaitWorkspaceTaskOptions } from './workspaceTaskWaiter'
