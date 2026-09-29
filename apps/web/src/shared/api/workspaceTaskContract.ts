// Workspace task contract mapping (issue #859).
//
// Feature-neutral contract seam split out of `workspaceTasks.ts`: status
// convergence, opaque-id asserts, canonical browser URLs, and thin
// snake_case transport types. Pure mapping only: no network, no SSE, no
// polling, no `window`. Transport lives in `workspaceTaskTransport.ts`,
// terminal waiting in `workspaceTaskWaiter.ts`.

import { requireContractData } from './contractKernel'
import { assertOpaqueId } from '../utils/opaqueIds'

export type WorkspaceTaskStatus = 'pending' | 'running' | 'completed' | 'failed' | 'cancelled'

const WORKSPACE_TASK_STATUSES: readonly WorkspaceTaskStatus[] = [
  'pending',
  'running',
  'completed',
  'failed',
  'cancelled',
]

export function mapWorkspaceTaskStatus(value: unknown): WorkspaceTaskStatus {
  if (typeof value === 'string') {
    // cancelled_with_results converges to cancelled for the v1 surface
    // (outputs carry the results).
    if (value === 'cancelled_with_results') return 'cancelled'
    if ((WORKSPACE_TASK_STATUSES as readonly string[]).includes(value)) {
      return value as WorkspaceTaskStatus
    }
  }
  throw new Error(`Unknown workspace task status: ${String(value)}`)
}

export function isWorkspaceTaskTerminal(status: WorkspaceTaskStatus): boolean {
  return status === 'completed' || status === 'failed' || status === 'cancelled'
}

export function assertWorkspaceTaskWorkspaceId(value: string): string {
  return assertOpaqueId(value, 'ws', 'workspace id')
}

export function assertWorkspaceTaskId(value: string): string {
  return assertOpaqueId(value, 'task', 'task id')
}

export function assertWorkspaceTaskAssetId(value: string): string {
  return assertOpaqueId(value, 'asset', 'asset id')
}

// Canonical browser URLs (full `/api/v1/...` form for Location handling
// and link sharing). The server `Location` header on 202 carries the same
// shape; clients derive it from the returned id when headers are opaque.
// No generated equivalent (orval URL helpers return bare `/workspaces/...`
// paths without the `/api/v1` prefix or Location semantics).
export function workspaceTaskLocation(workspaceId: string, taskId: string): string {
  return `/api/v1/workspaces/${encodeURIComponent(workspaceId)}/tasks/${encodeURIComponent(taskId)}`
}

export type WorkspaceTaskOutputRef = {
  kind: 'asset' | 'remote'
  asset_id?: string | undefined
  url?: string | undefined
  checksum_sha256?: string | undefined
  role?: string | undefined
  result_index: number
}

export type WorkspaceTaskBillingSnapshot = {
  sku: string
  display_name: string
  price_version_id: number
  price_version: number
  output_resolution: string
  unit_credits: number
  quantity: number
  total_credits: number
  quoted_at: string
  hold_transaction_id?: string | undefined
  settlement_state: string
}

export type WorkspaceTaskItem = {
  id: string
  workspace_id: string
  service: string
  media_asset_ids?: string[] | undefined
  status: WorkspaceTaskStatus
  retry_count: number
  execution_token?: string | undefined
  output_refs?: WorkspaceTaskOutputRef[] | undefined
  billing_snapshot: WorkspaceTaskBillingSnapshot
  error_msg?: string | undefined
  cancel_reason?: string | undefined
  created_at: string
  updated_at: string
  completed_at?: string | null | undefined
}

export type CreateWorkspaceTaskInput = {
  service: string
  media_asset_ids?: string[] | undefined
  /**
   * Free-text prompt (v1 `data.prompt`, max 8KB bytes). Required for
   * generation/inpaint, optional for render/variations/fusion/outpaint,
   * never sent for cutout/upscale (see the canvas adapter prompt policy).
   */
  prompt?: string | undefined
  params?: Record<string, unknown> | undefined
  quantity?: number | undefined
  resolution?: string | undefined
}

export type WorkspaceTasksRequestOptions = {
  signal?: AbortSignal | null | undefined
  requestId?: string | undefined
  idempotencyKey?: string | undefined
}

export function toWorkspaceTask(raw: unknown): WorkspaceTaskItem {
  const item = requireContractData<WorkspaceTaskItem>(raw, 'Task load failed') as WorkspaceTaskItem
  // Thin validation: status mapping throws on unknown states; ids are
  // shape-checked so enumeration feedback never leaks.
  mapWorkspaceTaskStatus((item as Record<string, unknown>).status)
  assertWorkspaceTaskId(item.id)
  assertWorkspaceTaskWorkspaceId(item.workspace_id)
  return item
}

export function toWorkspaceTaskOutputs(raw: unknown): {
  task_id: string
  outputs: WorkspaceTaskOutputRef[]
} {
  const data = requireContractData<{ task_id: string; outputs: WorkspaceTaskOutputRef[] }>(
    raw,
    'Task outputs load failed',
  )
  assertWorkspaceTaskId(data.task_id)
  const outputs = Array.isArray(data.outputs) ? data.outputs : []
  for (const ref of outputs) {
    if (ref.kind !== 'asset' && ref.kind !== 'remote') {
      throw new Error(`Unknown task output kind: ${String((ref as Record<string, unknown>).kind)}`)
    }
    if (ref.kind === 'asset') {
      assertWorkspaceTaskAssetId(ref.asset_id ?? '')
    }
    if (ref.kind === 'remote' && typeof ref.url === 'string') {
      const lowered = ref.url.toLowerCase()
      for (const marker of ['ws-uploads', 'builtin-local', 'storage_key']) {
        if (lowered.includes(marker)) throw new Error('Task output leaks storage internals')
      }
    }
  }
  return { task_id: data.task_id, outputs }
}
