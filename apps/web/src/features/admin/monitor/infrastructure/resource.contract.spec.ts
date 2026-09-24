import { describe, expect, it } from 'vitest'
import { http, HttpResponse, useMswHandlers } from '../../../../test/msw'
import * as monitorResource from './resource'
import { cancelAdminTaskByID, getAdminTaskDetail, getAdminTaskList } from './resource'

describe('task monitor resource', () => {
  it('does not expose physical task deletion through the admin task resource seam', () => {
    expect(monitorResource).not.toHaveProperty('deleteAdminTaskByID')
    expect(monitorResource).not.toHaveProperty('removeAdminTask')
  })

  it('cancels one active task through the typed admin task resource seam', async () => {
    const requests: Array<{ method: string; path: string; idempotencyKey: string | null }> = []
    useMswHandlers(
      http.post('/api/admin/tasks/task-a/cancel', ({ request }) => {
        requests.push({
          method: request.method,
          path: new URL(request.url).pathname,
          idempotencyKey: request.headers.get('Idempotency-Key'),
        })
        return HttpResponse.json({ code: 0, msg: 'ok', data: { message: '取消成功' } })
      }),
    )

    await expect(cancelAdminTaskByID('task-a')).resolves.toBeUndefined()
    expect(requests).toEqual([
      {
        method: 'POST',
        path: '/api/admin/tasks/task-a/cancel',
        idempotencyKey: 'task-cancel:task-a',
      },
    ])
  })

  it('loads the lightweight latest execution projection without execution history through the list resource seam', async () => {
    useMswHandlers(
      http.get('/api/admin/tasks', () =>
        HttpResponse.json({
          code: 0,
          msg: 'ok',
          data: {
            list: [
              {
                id: 1,
                task_id: 'task-a',
                correlation_id: 'task-a',
                user_public_id: '00000000-0000-0000-0000-000000000001',
                user_display_name: 'User 1',
                priority: 2,
                status: 'completed',
                created_at: '2026-07-27T08:00:00Z',
                updated_at: '2026-07-27T08:00:01Z',
                latest_execution: {
                  provider: 'openai',
                  gateway_step: 'text',
                  resolved_model: 'image-model',
                },
              },
              {
                id: 2,
                task_id: 'task-pending',
                correlation_id: 'task-pending',
                user_public_id: '00000000-0000-0000-0000-000000000001',
                user_display_name: 'User 1',
                priority: 2,
                status: 'pending',
                created_at: '2026-07-27T08:00:00Z',
                updated_at: '2026-07-27T08:00:00Z',
              },
            ],
            total: 2,
            pending_count: 1,
            running_count: 0,
            completed_count: 1,
            failed_count: 0,
            cancelled_count: 0,
            other_count: 0,
          },
        }),
      ),
    )

    const data = await getAdminTaskList({ page: 1, page_size: 20 })

    expect(data.list[0]?.latest_execution).toEqual({
      provider: 'openai',
      gateway_step: 'text',
      resolved_model: 'image-model',
    })
    expect(data.list[0]).not.toHaveProperty('execution_attempts')
    expect(data.list[1]?.latest_execution).toBeUndefined()
  })

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
              user_public_id: '00000000-0000-0000-0000-000000000001',
              user_display_name: 'User 1',
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
                  provider: 'openai',
                  gateway_step: 'text',
                  resolved_model: 'image-model',
                  provider_started_at: '2026-07-27T08:00:00Z',
                  provider_finished_at: '2026-07-27T08:00:01Z',
                  completed_at: '2026-07-27T08:00:01Z',
                  cancelled_at: null,
                  provider_latency_ms: 900,
                  total_latency_ms: 1000,
                },
              ],
              latest_execution: {
                provider: 'openai',
                gateway_step: 'text',
                resolved_model: 'image-model',
              },
            },
          },
        }),
      ),
    )

    const task = await getAdminTaskDetail('task-a')

    expect(task.task_id).toBe('task-a')
    expect(task.execution_attempts?.[0]).toMatchObject({
      provider: 'openai',
      resolved_model: 'image-model',
    })
    expect(task.latest_execution).toMatchObject({
      provider: 'openai',
      resolved_model: 'image-model',
    })
  })
})
