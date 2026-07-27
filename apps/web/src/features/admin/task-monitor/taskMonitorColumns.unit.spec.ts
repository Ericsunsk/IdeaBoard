import { describe, expect, it } from 'vitest'
import type { Task } from '../../../shared/api/task'
import { getTaskModelLabel } from './taskMonitorColumns'

describe('task monitor model column', () => {
  it('shows only the model resolved by runtime assignment', () => {
    expect(getTaskModelLabel(makeTask({ resolved_model: 'imagen-4' }))).toBe('imagen-4')
    expect(getTaskModelLabel(makeTask())).toBe('')
  })
})

function makeTask(overrides: Partial<Task> = {}): Task {
  return {
    id: 1,
    task_id: 'task-1',
    correlation_id: 'correlation-1',
    user_id: 1,
    prompt: 'prompt',
    status: 'pending',
    created_at: '2026-07-27T00:00:00Z',
    updated_at: '2026-07-27T00:00:00Z',
    ...overrides,
  }
}
