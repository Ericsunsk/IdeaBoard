import { describe, expect, it } from 'vitest'
import type { Task } from '../../../shared/api/task'
import {
  formatTaskDuration,
  formatTaskResultDebug,
  formatTaskResultSummary,
  getTaskApiCall,
  getTaskDuration,
  getTaskErrorReason,
  getTaskResultImageUrl,
  getTaskSanitizedError,
} from './taskResultPresentation'
import { adminTaskStatuses } from './taskMonitorStatus'

describe('task result presentation', () => {
  it('summarizes result payloads without exposing sensitive debug fields', () => {
    const summary = JSON.parse(
      formatTaskResultSummary(
        JSON.stringify({
          status: 'completed',
          platform: 'openai',
          images: ['/uploads/tasks/1/task/a.png', '/uploads/tasks/1/task/b.png'],
          token: 'secret-token',
        }),
      ),
    )
    const debug = JSON.parse(
      formatTaskResultDebug(
        JSON.stringify({
          status: 'completed',
          nested: {
            api_key: 'secret-key',
          },
        }),
      ),
    )

    expect(summary).toMatchObject({
      status: 'completed',
      platform: 'openai',
      images: 2,
    })
    expect(JSON.stringify(summary)).not.toContain('secret-token')
    expect(debug.nested.api_key).toBe('[REDACTED]')
  })

  it('derives result metadata used by monitor rows', () => {
    const task = makeTask({
      status: adminTaskStatuses.completed,
      result: JSON.stringify({
        platform: 'google',
        image_url: '/uploads/tasks/1/task/output.png',
      }),
    })

    expect(getTaskApiCall(task)).toEqual({ platform: 'google' })
    expect(getTaskResultImageUrl(task)).toBe('/uploads/tasks/1/task/output.png')
  })

  it('formats duration and tone consistently', () => {
    const task = makeTask({
      created_at: '2026-07-06T00:00:00Z',
      completed_at: '2026-07-06T00:01:01Z',
    })

    expect(getTaskDuration(task)).toEqual({ seconds: 61, tone: 'danger' })
    expect(formatTaskDuration(task)).toBe('61s')
  })

  it('sanitizes failed task errors from explicit and result payload sources', () => {
    const failedFromError = makeTask({
      status: adminTaskStatuses.failed,
      error_msg: 'provider failed with token=secret',
    })
    const failedFromResult = makeTask({
      status: adminTaskStatuses.failed,
      result: 'provider failed with token=secret',
    })

    expect(getTaskSanitizedError(failedFromError)).not.toContain('secret')
    expect(getTaskErrorReason(failedFromResult)).not.toContain('secret')
  })
})

function makeTask(overrides: Partial<Task>): Task {
  return {
    id: 1,
    task_id: 'task_1',
    user_id: 1,
    prompt: 'Prompt',
    status: adminTaskStatuses.completed,
    created_at: '2026-07-06T00:00:00Z',
    updated_at: '2026-07-06T00:00:00Z',
    ...overrides,
  }
}
