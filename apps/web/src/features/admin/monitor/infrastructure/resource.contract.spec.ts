import { describe, expect, it, vi } from 'vitest'
import * as monitorResource from './resource'
import {
  ADMIN_MONITOR_QUERY_KEY,
  ADMIN_TASK_LIST_QUERY_KEY,
  cancelAdminTaskByID,
  getAdminTaskDetail,
  getAdminTaskList,
  getAdminTaskDetailQueryKey,
  getAdminTaskListQueryKey,
  getAdminMonitorQueryKey,
} from './resource'
import { ADMIN_CONTROL_UNAVAILABLE_CODE } from '../../shared/lib/controlGuidance'

const transport = vi.hoisted(() => ({
  request: vi.fn(),
}))

vi.mock('../../../../shared/api/client', () => ({
  api: { request: transport.request },
}))

// Task list/detail/cancel have no equivalent Control read model (the
// control task dialect is a thin projection and its routes fail closed
// server-side until the workspace task read model lands), so they fail
// closed with guidance instead of calling the deleted `/api/admin/tasks*`
// routes. The monitor snapshot read stays on control (covered by the
// statistics/monitor specs).
describe('task monitor resource', () => {
  it('does not expose physical task deletion through the admin task resource seam', () => {
    expect(monitorResource).not.toHaveProperty('deleteAdminTaskByID')
    expect(monitorResource).not.toHaveProperty('removeAdminTask')
  })

  it('keeps the admin task query keys stable for invalidation', () => {
    expect(getAdminTaskListQueryKey()).toEqual(ADMIN_TASK_LIST_QUERY_KEY)
    expect(getAdminTaskDetailQueryKey('task-a')).toEqual(['admin', 'tasks', 'task-a'])
    expect(getAdminMonitorQueryKey()).toEqual(ADMIN_MONITOR_QUERY_KEY)
  })

  it('fails the task list closed with guidance and no transport call', async () => {
    await expect(getAdminTaskList({ page: 1, page_size: 20 })).rejects.toMatchObject({
      code: ADMIN_CONTROL_UNAVAILABLE_CODE,
    })
    expect(transport.request).not.toHaveBeenCalled()
  })

  it('fails the task detail closed with guidance and no transport call', async () => {
    await expect(getAdminTaskDetail('task-a')).rejects.toMatchObject({
      code: ADMIN_CONTROL_UNAVAILABLE_CODE,
    })
    expect(transport.request).not.toHaveBeenCalled()
  })

  it('fails task cancellation closed with guidance and no transport call', async () => {
    await expect(cancelAdminTaskByID('task-a')).rejects.toMatchObject({
      code: ADMIN_CONTROL_UNAVAILABLE_CODE,
    })
    expect(transport.request).not.toHaveBeenCalled()
  })
})
