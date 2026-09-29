// Issue #805: workspace task lifecycle contract spec.
//
// Covers the discriminated input union validation mapping, cancel
// Idempotency-Key headers, terminal-state mapping, output-ref adapter
// (asset vs remote, no storage internals), and the 404-vs-403 branches.
// MSW only by design (contract specs must not mock generated-query
// modules); the legacy `/api/user` task transport is retired (W2 stream 1:
// canvas migrates onto this surface).
import { describe, expect, it } from 'vitest'
import { http, HttpResponse, useMswHandlers } from '../../test/msw'
import {
  assertWorkspaceTaskAssetId,
  assertWorkspaceTaskId,
  assertWorkspaceTaskWorkspaceId,
  cancelWorkspaceTask,
  createWorkspaceTask,
  getWorkspaceTask,
  isWorkspaceTaskTerminal,
  listWorkspaceTaskOutputs,
  listWorkspaceTasks,
  mapWorkspaceTaskStatus,
  quoteWorkspaceTask,
  toWorkspaceTask,
  toWorkspaceTaskOutputs,
  workspaceTaskLocation,
} from './workspaceTasks'
import {
  getCancelWorkspaceTaskUrl,
  getCreateWorkspaceTaskUrl,
  getGetWorkspaceTaskUrl,
  getListWorkspaceTaskOutputsUrl,
  getListWorkspaceTasksUrl,
  getQuoteWorkspaceTaskUrl,
} from './generated-query/public/workspace-tasks/workspace-tasks'
import { workspaceTaskIdempotencyKey } from './idempotencyKey'

const WORKSPACE_ID = 'ws_abc123'
const TASK_ID = 'task_def456'
const ASSET_ID = 'asset_789abc'
const CHECKSUM = 'e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855'

function taskFixture(overrides: Record<string, unknown> = {}): Record<string, unknown> {
  return {
    data: {
      id: TASK_ID,
      workspace_id: WORKSPACE_ID,
      service: 'render',
      media_asset_ids: [],
      status: 'pending',
      retry_count: 0,
      execution_token: 'exec_abc',
      output_refs: [],
      billing_snapshot: {
        sku: 'ai-image:render',
        display_name: 'Render',
        price_version_id: 1,
        price_version: 1,
        output_resolution: '1k',
        unit_credits: 10,
        quantity: 1,
        total_credits: 10,
        quoted_at: '2026-09-23T00:00:00Z',
        hold_transaction_id: 'hold_task_def456',
        settlement_state: 'held',
      },
      created_at: '2026-09-23T00:00:00Z',
      updated_at: '2026-09-23T00:00:00Z',
      ...overrides,
    },
  }
}

describe('workspace task paths', () => {
  it('builds canonical routes matching the generated hooks (publicMutator adds /v1)', () => {
    expect(getListWorkspaceTasksUrl(WORKSPACE_ID)).toBe('/workspaces/ws_abc123/tasks')
    expect(getCreateWorkspaceTaskUrl(WORKSPACE_ID)).toBe('/workspaces/ws_abc123/tasks')
    expect(getGetWorkspaceTaskUrl(WORKSPACE_ID, TASK_ID)).toBe(
      '/workspaces/ws_abc123/tasks/task_def456',
    )
    expect(getCancelWorkspaceTaskUrl(WORKSPACE_ID, TASK_ID)).toBe(
      '/workspaces/ws_abc123/tasks/task_def456/cancel',
    )
    expect(getListWorkspaceTaskOutputsUrl(WORKSPACE_ID, TASK_ID)).toBe(
      '/workspaces/ws_abc123/tasks/task_def456/outputs',
    )
    expect(getQuoteWorkspaceTaskUrl(WORKSPACE_ID)).toBe('/workspaces/ws_abc123/task-quotes')
    expect(workspaceTaskLocation(WORKSPACE_ID, TASK_ID)).toBe(
      '/api/v1/workspaces/ws_abc123/tasks/task_def456',
    )
  })

  it('rejects malformed opaque ids before any network call', async () => {
    expect(() => assertWorkspaceTaskWorkspaceId('nope')).toThrow()
    expect(() => assertWorkspaceTaskId('upload_x')).toThrow()
    expect(() => assertWorkspaceTaskAssetId('42')).toThrow()
    await expect(listWorkspaceTasks('  ')).rejects.toThrow()
    await expect(getWorkspaceTask(WORKSPACE_ID, 'task with space')).rejects.toThrow()
  })
})

describe('workspace task union validation mapping', () => {
  it('maps every terminal state and converges cancelled_with_results', () => {
    for (const status of ['pending', 'running', 'completed', 'failed', 'cancelled']) {
      expect(mapWorkspaceTaskStatus(status)).toBe(status)
    }
    expect(mapWorkspaceTaskStatus('cancelled_with_results')).toBe('cancelled')
    expect(() => mapWorkspaceTaskStatus('ready')).toThrow()
    expect(() => mapWorkspaceTaskStatus(null)).toThrow()
    expect(isWorkspaceTaskTerminal('completed')).toBe(true)
    expect(isWorkspaceTaskTerminal('failed')).toBe(true)
    expect(isWorkspaceTaskTerminal('cancelled')).toBe(true)
    expect(isWorkspaceTaskTerminal('pending')).toBe(false)
    expect(isWorkspaceTaskTerminal('running')).toBe(false)
  })

  it('parses task payloads and guards opaque ids', () => {
    const task = toWorkspaceTask(taskFixture())
    expect(task.id).toBe(TASK_ID)
    expect(task.status).toBe('pending')
    expect(() =>
      toWorkspaceTask({
        data: { ...(taskFixture().data as Record<string, unknown>), status: 'ready' },
      }),
    ).toThrow()
  })

  it('rejects output refs that leak storage internals', () => {
    const good = toWorkspaceTaskOutputs({
      data: {
        task_id: TASK_ID,
        outputs: [
          { kind: 'asset', asset_id: ASSET_ID, result_index: 0 },
          {
            kind: 'remote',
            url: 'https://cdn.example.com/o.png',
            checksum_sha256: CHECKSUM,
            result_index: 1,
          },
        ],
      },
    })
    expect(good.outputs).toHaveLength(2)
    expect(() =>
      toWorkspaceTaskOutputs({
        data: {
          task_id: TASK_ID,
          outputs: [
            {
              kind: 'remote',
              url: 'https://x/ws-uploads/y',
              checksum_sha256: CHECKSUM,
              result_index: 0,
            },
          ],
        },
      }),
    ).toThrow()
    expect(() =>
      toWorkspaceTaskOutputs({
        data: { task_id: TASK_ID, outputs: [{ kind: 'mystery', result_index: 0 }] },
      }),
    ).toThrow()
  })
})

describe('workspace task cancel headers', () => {
  it('sends Idempotency-Key on create and cancel (header-only scope task:create/task:cancel)', async () => {
    const seen: Record<string, string | null> = {}
    useMswHandlers(
      http.post('/api/v1/workspaces/:workspaceId/tasks', async ({ request }) => {
        seen.create = request.headers.get('Idempotency-Key')
        return HttpResponse.json(taskFixture(), { status: 202 })
      }),
      http.post('/api/v1/workspaces/:workspaceId/tasks/:taskId/cancel', async ({ request }) => {
        seen.cancel = request.headers.get('Idempotency-Key')
        return HttpResponse.json(taskFixture({ status: 'cancelled' }))
      }),
    )
    const createKey = workspaceTaskIdempotencyKey('create')
    await createWorkspaceTask(WORKSPACE_ID, { service: 'render' }, { idempotencyKey: createKey })
    expect(seen.create).toBe(createKey)
    const cancelKey = workspaceTaskIdempotencyKey('cancel')
    await cancelWorkspaceTask(
      WORKSPACE_ID,
      TASK_ID,
      { reason: 'stop' },
      { idempotencyKey: cancelKey },
    )
    expect(seen.cancel).toBe(cancelKey)
  })

  it('surfaces 409 task_idempotency_conflict on key reuse with a different hash', async () => {
    useMswHandlers(
      http.post('/api/v1/workspaces/:workspaceId/tasks', () => {
        return HttpResponse.json(
          {
            error: { code: 'task_idempotency_conflict', message: 'conflict', request_id: 'req_1' },
          },
          { status: 409 },
        )
      }),
    )
    await expect(
      createWorkspaceTask(WORKSPACE_ID, { service: 'render' }, { idempotencyKey: 'k' }),
    ).rejects.toMatchObject({ code: 'task_idempotency_conflict' })
  })

  it('sends data.prompt and params.mask_asset_id in the v1 create body', async () => {
    const seen: Array<Record<string, unknown>> = []
    useMswHandlers(
      http.post('/api/v1/workspaces/:workspaceId/tasks', async ({ request }) => {
        seen.push((await request.json()) as Record<string, unknown>)
        return HttpResponse.json(taskFixture(), { status: 202 })
      }),
    )
    await createWorkspaceTask(
      WORKSPACE_ID,
      {
        service: 'inpaint',
        media_asset_ids: [ASSET_ID],
        prompt: 'redraw the area',
        params: { mask_asset_id: ASSET_ID, use_mask: true },
      },
      { idempotencyKey: workspaceTaskIdempotencyKey('create') },
    )
    expect(seen).toHaveLength(1)
    expect(seen[0]).toMatchObject({
      data: {
        service: 'inpaint',
        inputs: { media_asset_ids: [ASSET_ID] },
        prompt: 'redraw the area',
        params: { mask_asset_id: ASSET_ID, use_mask: true },
      },
    })
  })

  it('omits data.prompt when the input carries none', async () => {
    const seen: Array<Record<string, unknown>> = []
    useMswHandlers(
      http.post('/api/v1/workspaces/:workspaceId/tasks', async ({ request }) => {
        seen.push((await request.json()) as Record<string, unknown>)
        return HttpResponse.json(taskFixture(), { status: 202 })
      }),
    )
    await createWorkspaceTask(WORKSPACE_ID, { service: 'upscale' })
    expect((seen[0] as { data: Record<string, unknown> }).data).not.toHaveProperty('prompt')
  })
})

describe('workspace task 404/403 matrix', () => {
  it('maps hidden tasks to task_not_found and forbidden to task_forbidden', async () => {
    useMswHandlers(
      http.get('/api/v1/workspaces/:workspaceId/tasks/:taskId', ({ params }) => {
        const taskId = String(params.taskId ?? '')
        if (taskId === TASK_ID) return HttpResponse.json(taskFixture())
        return HttpResponse.json(
          { error: { code: 'task_not_found', message: 'missing', request_id: 'req_1' } },
          { status: 404 },
        )
      }),
      http.get('/api/v1/workspaces/:workspaceId/tasks/:taskId/outputs', () => {
        return HttpResponse.json(
          { error: { code: 'task_not_found', message: 'missing', request_id: 'req_1' } },
          { status: 404 },
        )
      }),
    )
    await expect(getWorkspaceTask(WORKSPACE_ID, TASK_ID)).resolves.toMatchObject({ id: TASK_ID })
    await expect(getWorkspaceTask(WORKSPACE_ID, 'task_missing000')).rejects.toMatchObject({
      code: 'task_not_found',
    })
    await expect(listWorkspaceTaskOutputs(WORKSPACE_ID, TASK_ID)).rejects.toMatchObject({
      code: 'task_not_found',
    })
  })

  it('lists with cursor pagination and quotes without persistence', async () => {
    useMswHandlers(
      http.get('/api/v1/workspaces/:workspaceId/tasks', ({ request }) => {
        const url = new URL(request.url)
        expect(url.searchParams.get('page_size')).toBe('2')
        return HttpResponse.json({ data: [], meta: { next_cursor: '', has_more: false } })
      }),
      http.post('/api/v1/workspaces/:workspaceId/task-quotes', () => {
        return HttpResponse.json({
          data: {
            sku: 'ai-image:render',
            display_name: 'Render',
            price_version_id: 1,
            price_version: 1,
            output_resolution: '1k',
            unit_credits: 10,
            quantity: 1,
            total_credits: 10,
            quoted_at: '2026-09-23T00:00:00Z',
            settlement_state: 'quoted',
          },
        })
      }),
    )
    await listWorkspaceTasks(WORKSPACE_ID, { pageSize: 2 })
    const quote = await quoteWorkspaceTask(WORKSPACE_ID, { service: 'render' })
    expect(quote.settlement_state).toBe('quoted')
    expect(getCreateWorkspaceTaskUrl(WORKSPACE_ID)).toBe('/workspaces/ws_abc123/tasks')
  })
})
