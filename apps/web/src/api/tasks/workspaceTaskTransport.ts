// Workspace task transport (issue #859).
//
// Feature-neutral CRUD + quote transport split out of `workspaceTasks.ts`.
// Rides `publicMutator` (`/api/v1/...`) through the generated functions;
// keeps the opaque-id asserts, header shaping, and storage-leak guards that
// have no generated equivalent. No SSE, no polling, no `window`: terminal
// waiting lives in `workspaceTaskWaiter.ts`.

import {
  cancelWorkspaceTask as cancelWorkspaceTaskTransport,
  createWorkspaceTask as createWorkspaceTaskTransport,
  getWorkspaceTask as getWorkspaceTaskTransport,
  listWorkspaceTaskOutputs as listWorkspaceTaskOutputsTransport,
  listWorkspaceTasks as listWorkspaceTasksTransport,
  quoteWorkspaceTask as quoteWorkspaceTaskTransport,
} from '../generated-query/public/workspace-tasks/workspace-tasks'
import type {
  CreateWorkspaceTaskRequest,
  PaginatedWorkspaceTasks,
  QuoteWorkspaceTaskRequest,
} from '../generated-query/public/model'
import { buildContractRequestHeaders, newRequestId, requireContractData } from '../http/contractKernel'
import { PAGE_SIZES } from '../../core/config/pagination'
import {
  assertWorkspaceTaskId,
  assertWorkspaceTaskWorkspaceId,
  toWorkspaceTask,
  toWorkspaceTaskOutputs,
  type CreateWorkspaceTaskInput,
  type WorkspaceTaskBillingSnapshot,
  type WorkspaceTaskItem,
  type WorkspaceTaskOutputRef,
  type WorkspaceTasksRequestOptions,
} from './workspaceTaskContract'
import { withSignal } from '../../core/utils/request'

function toGeneratedOptions(options: WorkspaceTasksRequestOptions | undefined): {
  headers: Record<string, string>
  signal?: AbortSignal | undefined
} {
  return {
    headers: buildContractRequestHeaders({
      requestId: options?.requestId?.trim() ? options.requestId.trim() : newRequestId(),
      ...(options?.idempotencyKey == null ? {} : { idempotencyKey: options.idempotencyKey }),
    }),
    ...withSignal(options?.signal),
  }
}

/**
 * Shared transport-error inspection for task wait/cancel paths. Gone (404
 * converged hidden) settles silently; callers map the conflict/terminal
 * branches. Kept here so the SSE/poll waiter and the canvas cancel/binding
 * seam converge on one predicate instead of drifting copies.
 */
export function isWorkspaceTaskGoneError(error: unknown): boolean {
  if (typeof error !== 'object' || error === null) return false
  const status = 'status' in error ? (error as { status?: unknown }).status : undefined
  if (status === 404) return true
  const code = 'code' in error ? (error as { code?: unknown }).code : undefined
  return code === 'task_not_found' || code === 404
}

export async function listWorkspaceTasks(
  workspaceId: string,
  params?: { cursor?: string | null | undefined; pageSize?: number | undefined },
  options?: WorkspaceTasksRequestOptions,
): Promise<PaginatedWorkspaceTasks> {
  const query: { cursor?: string; page_size?: number } = {
    page_size: params?.pageSize ?? PAGE_SIZES.standard,
  }
  if (params?.cursor?.trim()) query.cursor = params.cursor.trim()
  return listWorkspaceTasksTransport(
    assertWorkspaceTaskWorkspaceId(workspaceId),
    query,
    toGeneratedOptions(options),
  )
}

export async function createWorkspaceTask(
  workspaceId: string,
  input: CreateWorkspaceTaskInput,
  options?: WorkspaceTasksRequestOptions,
): Promise<WorkspaceTaskItem> {
  const data: CreateWorkspaceTaskRequest['data'] = {
    service: input.service as CreateWorkspaceTaskRequest['data']['service'],
    inputs: { media_asset_ids: input.media_asset_ids ?? [] },
  }
  if (input.prompt != null) data.prompt = input.prompt
  if (input.params != null) {
    data.params = input.params as NonNullable<CreateWorkspaceTaskRequest['data']['params']>
  }
  if (input.quantity != null) data.quantity = input.quantity
  if (input.resolution != null) data.resolution = input.resolution
  if (input.board_id != null) data.board_id = input.board_id
  const envelope = await createWorkspaceTaskTransport(
    assertWorkspaceTaskWorkspaceId(workspaceId),
    { data },
    toGeneratedOptions(options),
  )
  // 202 Accepted carries the same envelope as 200; the Location header
  // (`/api/v1/workspaces/{ws}/tasks/{id}`) is authoritative when exposed.
  return toWorkspaceTask(envelope)
}

export async function getWorkspaceTask(
  workspaceId: string,
  taskId: string,
  options?: WorkspaceTasksRequestOptions,
): Promise<WorkspaceTaskItem> {
  const envelope = await getWorkspaceTaskTransport(
    assertWorkspaceTaskWorkspaceId(workspaceId),
    assertWorkspaceTaskId(taskId),
    toGeneratedOptions(options),
  )
  return toWorkspaceTask(envelope)
}

export async function cancelWorkspaceTask(
  workspaceId: string,
  taskId: string,
  input?: { reason?: string | undefined },
  options?: WorkspaceTasksRequestOptions,
): Promise<WorkspaceTaskItem> {
  const envelope = await cancelWorkspaceTaskTransport(
    assertWorkspaceTaskWorkspaceId(workspaceId),
    assertWorkspaceTaskId(taskId),
    input?.reason?.trim() ? { data: { reason: input.reason.trim() } } : undefined,
    toGeneratedOptions(options),
  )
  return toWorkspaceTask(envelope)
}

export async function listWorkspaceTaskOutputs(
  workspaceId: string,
  taskId: string,
  options?: WorkspaceTasksRequestOptions,
): Promise<{ task_id: string; outputs: WorkspaceTaskOutputRef[] }> {
  const envelope = await listWorkspaceTaskOutputsTransport(
    assertWorkspaceTaskWorkspaceId(workspaceId),
    assertWorkspaceTaskId(taskId),
    toGeneratedOptions(options),
  )
  return toWorkspaceTaskOutputs(envelope)
}

export async function quoteWorkspaceTask(
  workspaceId: string,
  input: {
    service: string
    quantity?: number | undefined
    resolution?: string | undefined
    board_id?: string | undefined
  },
  options?: WorkspaceTasksRequestOptions,
): Promise<WorkspaceTaskBillingSnapshot> {
  const data: QuoteWorkspaceTaskRequest['data'] = {
    service: input.service as QuoteWorkspaceTaskRequest['data']['service'],
  }
  if (input.quantity != null) data.quantity = input.quantity
  if (input.resolution != null) data.resolution = input.resolution
  if (input.board_id != null) data.board_id = input.board_id
  const envelope = await quoteWorkspaceTaskTransport(
    assertWorkspaceTaskWorkspaceId(workspaceId),
    { data },
    toGeneratedOptions(options),
  )
  return requireContractData<WorkspaceTaskBillingSnapshot>(envelope, 'Quote failed')
}
