// Canvas workspace task port (Public Data Plane v1).
//
// Stable task port for Canvas, Workspace, and Admin callers (issue #859):
// external modules depend only on this seam, never on the transport, SSE,
// or Vue lifecycle implementations behind it. Each lifecycle concern owns
// an independent module + test seam:
//
// - `canvasTaskContractMapping` — contract mapping (submit/quote inputs,
//   fail-closed guards; services/resolutions from the shared catalog).
// - `canvasTaskReadiness` — quote/readiness fetching.
// - `canvasTaskWaiting` — SSE/poll terminal waiting + result settlement.
// - `canvasTaskCancellation` — task -> workspace binding + cancel.
// - `task-lifecycle` — cancel retry policy owner (injects the cancel seam).
// - `billing/readiness` + `runtime/pendingWorkflowAssembly` — Vue
//   lifecycle adapters (provide/inject, watchEffect, scope disposal).
//
// Semantic mappings (#809 audit):
// - submit: `CanvasTaskRequestData` prompt/service/image inputs/params ->
//   `CreateWorkspaceTaskInput` service/media_asset_ids/prompt
//   (+params incl. `mask_asset_id`, quantity, resolution).
//   `client_action_id` becomes the header-only `Idempotency-Key` (never
//   sent in the v1 body); the prompt rides `data.prompt` and is covered
//   by the server idempotency hash (same key + different prompt -> 409).
// - readiness: `ready/message/billing.sufficient` <- task-quotes snapshot.
//   Quotes are price-only: no wallet balances ride the snapshot, so
//   `available_credits`/`reserved_credits` map to 0 and sufficiency is
//   inferred from the absence of 402 (documented below). There is no v1
//   matrix endpoint; matrix fans out to one quote per request.
// - wait: waiter image-URL extraction <- asset-ref outputs (`output_refs`).
//   Asset refs resolve through the media browser URL; remote refs resolve
//   direct. v1 tasks carry no `execution_metrics`, so the legacy
//   finalize-failure pending detection is unavailable (failed surfaces
//   immediately; documented residual).
// - cancel: needs the workspace id, resolved per task (see registry).
//
// Fail-closed rules: anything without a faithful v1 mapping throws
// `CanvasWorkspaceTaskUnmappableError` BEFORE any network call (never a
// silent drop, never a cryptic 422). Missing workspace selection throws
// `CanvasWorkspaceUnavailableError` with guidance.

import { createWorkspaceTask } from '../../../../../api/workspaceTaskTransport'
import { workspaceTaskIdempotencyKey } from '../../../../../api/idempotencyKey'
import { notifyUserCreditsInvalidation } from '../../../../../core/query/userCreditsInvalidation'
import type { CanvasTaskRequestData } from '../../../core/execution/canvasTaskRequest'
import { toWorkspaceCreateInput } from './canvasTaskContractMapping'
import { registerCanvasWorkspaceTask, requireCanvasWorkspaceId } from './canvasTaskCancellation'

export {
  CanvasWorkspaceTaskUnmappableError,
  isCanvasWorkspaceTaskUnmappableError,
  normalizeCanvasOutputResolution,
  toWorkspaceCreateInput,
  toWorkspaceQuoteInput,
} from './canvasTaskContractMapping'
export {
  CANVAS_WORKSPACE_REQUIRED_MESSAGE,
  CanvasWorkspaceUnavailableError,
  isCanvasWorkspaceUnavailableError,
  cancelCanvasWorkspaceTask,
  registerCanvasWorkspaceTask,
  requireCanvasWorkspaceId,
  resetCanvasWorkspaceTaskRegistryForTests,
} from './canvasTaskCancellation'
export {
  fetchCanvasWorkspaceTaskReadiness,
  fetchCanvasWorkspaceTaskReadinessMatrix,
  toSubmitTaskReadiness,
} from './canvasTaskReadiness'
export {
  extractWorkspaceTaskImageResults,
  waitCanvasWorkspaceTaskImageResults,
} from './canvasTaskWaiting'

export async function submitCanvasWorkspaceTask(
  request: CanvasTaskRequestData,
  options?: { idempotencyKey?: string | undefined },
): Promise<{ taskId: string; workspaceId: string }> {
  const workspaceId = requireCanvasWorkspaceId()
  const input = toWorkspaceCreateInput(request)
  const clientActionId = `${request.client_action_id ?? ''}`.trim()
  const idempotencyKey =
    options?.idempotencyKey?.trim() || clientActionId || workspaceTaskIdempotencyKey('create')
  const item = await createWorkspaceTask(workspaceId, input, { idempotencyKey })
  registerCanvasWorkspaceTask(item.id, item.workspace_id)
  notifyUserCreditsInvalidation({ source: 'task-hold', taskId: item.id })
  return { taskId: item.id, workspaceId: item.workspace_id }
}
