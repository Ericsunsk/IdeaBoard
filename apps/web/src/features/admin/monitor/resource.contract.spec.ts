import { beforeEach, describe, expect, it, vi } from 'vitest'
import * as monitorResource from './resource'
import {
  ADMIN_MONITOR_QUERY_KEY,
  ADMIN_TASK_LIST_QUERY_KEY,
  cancelAdminTaskByID,
  encodeTaskListCursor,
  getAdminTaskDetail,
  getAdminTaskList,
  getAdminTaskDetailQueryKey,
  getAdminTaskListQueryKey,
  getAdminMonitorQueryKey,
} from './resource'
import { ADMIN_CONTROL_UNAVAILABLE_CODE } from '../shared'

const transport = vi.hoisted(() => ({
  request: vi.fn(),
}))

vi.mock('../../../api/client', () => ({
  api: { request: transport.request },
}))

// Task list/detail ride the live Control task read model (#852); only
// task cancellation still fails closed with guidance (mutations need an
// operator reason the monitor UI does not collect).
describe('task monitor resource', () => {
  beforeEach(() => {
    transport.request.mockClear()
  })
  it('does not expose physical task deletion through the admin task resource seam', () => {
    expect(monitorResource).not.toHaveProperty('deleteAdminTaskByID')
    expect(monitorResource).not.toHaveProperty('removeAdminTask')
  })

  it('keeps the admin task query keys stable for invalidation', () => {
    expect(getAdminTaskListQueryKey()).toEqual(ADMIN_TASK_LIST_QUERY_KEY)
    expect(getAdminTaskDetailQueryKey('task-a')).toEqual(['admin', 'monitor', 'detail', 'task-a'])
    expect(getAdminMonitorQueryKey()).toEqual(ADMIN_MONITOR_QUERY_KEY)
  })

  it('encodes page cursors the way the backend decodes them', () => {
    expect(encodeTaskListCursor(1)).toBeUndefined()
    expect(encodeTaskListCursor(0)).toBeUndefined()
    const cursor = encodeTaskListCursor(3)
    expect(typeof cursor).toBe('string')
    // base64url of {"page":3}, canonical (no padding, URL-safe).
    expect(cursor).toBe(btoa('{"page":3}').replace(/=+$/g, ''))
  })

  it('lists tasks through the generated control client with mapped params', async () => {
    transport.request.mockResolvedValueOnce({
      data: [
        {
          task_id: 'task-1',
          service: 'render',
          requested_resolution: '2k',
          pool_version: 3,
          selected_model: 'openrouter/model-a',
          final_resolved_model: 'openrouter/model-b',
          status: 'completed',
        },
      ],
      meta: {
        next_cursor: null,
        has_more: false,
        counts: {
          total: 1,
          pending_count: 0,
          running_count: 0,
          completed_count: 1,
          failed_count: 0,
          cancelled_count: 0,
          other_count: 0,
        },
      },
    })

    const page = await getAdminTaskList({
      status: 'completed',
      keyword: 'house',
      user_public_id: 'usr_abc',
      page: 2,
      page_size: 20,
    })

    expect(transport.request).toHaveBeenCalledTimes(1)
    const [url] = transport.request.mock.calls[0] as [string]
    expect(url.startsWith('/control/v1/tasks?')).toBe(true)
    expect(url).toContain('status=completed')
    expect(url).toContain('keyword=house')
    expect(url).toContain('user_id=usr_abc')
    expect(url).toContain(`cursor=${encodeTaskListCursor(2)}`)
    expect(page.total).toBe(1)
    expect(page.completed_count).toBe(1)
    expect(page.list).toHaveLength(1)
    expect(page.list[0]?.requested_resolution).toBe('2k')
    expect(page.list[0]?.pool_version).toBe(3)
    expect(page.list[0]?.selected_model).toBe('openrouter/model-a')
    expect(page.list[0]?.final_resolved_model).toBe('openrouter/model-b')
  })

  it('maps task detail routing, attempts, and final model', async () => {
    transport.request.mockResolvedValueOnce({
      data: {
        task: {
          task_id: 'task-1',
          service: 'render',
          status: 'completed',
          final_resolved_model: 'openrouter/model-b',
          routing: {
            service: 'render',
            requested_resolution: '2k',
            eligible_models: ['openrouter/model-a', 'openrouter/model-b'],
            selected_model: 'openrouter/model-a',
            retry_candidate: 'openrouter/model-b',
            pool_version: 3,
          },
          execution_attempts: [
            {
              attempt_no: 1,
              status: 'failed',
              retry_layer: 'gateway',
              gateway_retry_count: 2,
              failure_category: 'provider_error',
              requested_model: 'openrouter/model-a',
              resolved_model: 'openrouter/model-a',
            },
            {
              attempt_no: 2,
              status: 'completed',
              retry_layer: 'model_fallback',
              gateway_retry_count: 0,
              requested_model: 'openrouter/model-b',
              resolved_model: 'openrouter/model-b',
            },
          ],
        },
      },
    })

    const detail = await getAdminTaskDetail('task-1')

    expect(transport.request).toHaveBeenCalledTimes(1)
    const [url] = transport.request.mock.calls[0] as [string]
    expect(url).toBe('/control/v1/tasks/task-1')
    expect(detail.routing?.pool_version).toBe(3)
    expect(detail.routing?.selected_model).toBe('openrouter/model-a')
    expect(detail.final_resolved_model).toBe('openrouter/model-b')
    expect(detail.execution_attempts).toHaveLength(2)
    expect(detail.execution_attempts[0]?.retry_layer).toBe('gateway')
    expect(detail.execution_attempts[1]?.retry_layer).toBe('model_fallback')
  })

  it('fails task cancellation closed with guidance and no transport call', async () => {
    await expect(cancelAdminTaskByID('task-a')).rejects.toMatchObject({
      code: ADMIN_CONTROL_UNAVAILABLE_CODE,
    })
    expect(transport.request).not.toHaveBeenCalled()
  })
})
