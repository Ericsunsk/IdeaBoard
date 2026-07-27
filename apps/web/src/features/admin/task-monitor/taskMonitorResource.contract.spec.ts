import { describe, expect, it } from 'vitest'
import { http, HttpResponse, useMswHandlers } from '../../../test/msw'
import { deleteAdminTaskSelection, getAdminTaskDetail } from './taskMonitorResource'

describe('task monitor resource', () => {
  it('loads the Task-owned execution trace through the detail resource seam', async () => {
    useMswHandlers(
      http.get('/api/admin/tasks/task-a', () =>
        HttpResponse.json({
          code: 0,
          msg: 'ok',
          data: {
            task: {
              id: 1,
              task_id: 'task-a',
              correlation_id: 'task-a',
              user_id: 1,
              priority: 2,
              status: 'completed',
              created_at: '2026-07-27T08:00:00Z',
              updated_at: '2026-07-27T08:00:01Z',
              execution_attempts: [
                {
                  attempt_no: 1,
                  status: 'completed',
                  stage: 'completed',
                  failure_category: '',
                  provider_endpoint_id: 7,
                  provider_endpoint_name: 'Provider A',
                  resolved_model: 'image-model',
                  api_type: 'google-native',
                  provider_started_at: '2026-07-27T08:00:00Z',
                  provider_finished_at: '2026-07-27T08:00:01Z',
                  finalize_started_at: null,
                  finalize_finished_at: null,
                  completed_at: '2026-07-27T08:00:01Z',
                  cancelled_at: null,
                  provider_latency_ms: 900,
                  finalize_latency_ms: 0,
                  total_latency_ms: 1000,
                },
              ],
            },
          },
        }),
      ),
    )

    const task = await getAdminTaskDetail('task-a')

    expect(task.task_id).toBe('task-a')
    expect(task.execution_attempts?.[0]).toMatchObject({
      provider_endpoint_name: 'Provider A',
      resolved_model: 'image-model',
    })
  })

  it('deletes selected tasks through the admin task monitor resource seam', async () => {
    const requests: Array<{ method: string; path: string; body?: unknown }> = []
    useMswHandlers(
      http.post('/api/admin/tasks/bulk-delete', async ({ request }) => {
        const url = new URL(request.url)
        requests.push({
          method: request.method,
          path: url.pathname,
          body: await readRequestJson(request),
        })

        return HttpResponse.json({
          code: 0,
          msg: 'ok',
          data: { deleted_count: 2 },
        })
      }),
    )

    const result = await deleteAdminTaskSelection(['task-a', 'task-b'])

    expect(requests).toEqual([
      {
        method: 'POST',
        path: '/api/admin/tasks/bulk-delete',
        body: { task_ids: ['task-a', 'task-b'] },
      },
    ])
    expect(result.deleted_count).toBe(2)
  })
})

async function readRequestJson(request: Request) {
  const text = await request.text()
  return text ? JSON.parse(text) : undefined
}
