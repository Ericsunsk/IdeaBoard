import { describe, expect, it } from 'vitest'
import type { AdminTaskMonitorListItemModel } from './taskMonitorResource'
import {
  getTaskAPITypeLabel,
  getTaskAccountLabel,
  getTaskEndpointLabel,
  getTaskModelLabel,
} from './taskMonitorColumns'

describe('task monitor model column', () => {
  it('shows execution fields only from the immutable latest execution snapshot', () => {
    expect(getTaskModelLabel(makeTask())).toBe('')
    expect(getTaskEndpointLabel(makeTask())).toBe('')
    expect(getTaskAPITypeLabel(makeTask())).toBe('')
  })

  it('shows the lightweight latest immutable execution projection after a terminal Task releases its lease', () => {
    const task = makeTask({
      latest_execution: {
        account_id: 101,
        provider_endpoint_id: 202,
        api_type: 'google-native',
        provider_endpoint_name: 'Provider A',
        resolved_model: 'imagen-4',
      },
    })

    expect(getTaskEndpointLabel(task)).toBe('Provider A')
    expect(getTaskModelLabel(task)).toBe('imagen-4')
    expect(getTaskAPITypeLabel(task)).toBe('google-native')
    expect(getTaskAccountLabel(task)).toBe('101')
  })

  it('keeps pending Tasks without an attempt in an explicit empty state', () => {
    const task = makeTask()

    expect(getTaskEndpointLabel(task)).toBe('')
    expect(getTaskModelLabel(task)).toBe('')
    expect(getTaskAPITypeLabel(task)).toBe('')
  })
})

function makeTask(
  overrides: Partial<AdminTaskMonitorListItemModel> = {},
): AdminTaskMonitorListItemModel {
  return {
    task_id: 'task-1',
    user_id: 1,
    prompt: 'prompt',
    priority: 2,
    status: 'pending',
    error_msg: '',
    result: '',
    created_at: '2026-07-27T00:00:00Z',
    completed_at: null,
    ...overrides,
  }
}
