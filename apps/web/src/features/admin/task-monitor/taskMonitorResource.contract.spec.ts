import { describe, expect, it } from 'vitest'
import { http, HttpResponse, useMswHandlers } from '../../../test/msw'
import { deleteAdminTaskSelection } from './taskMonitorResource'

describe('task monitor resource', () => {
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
