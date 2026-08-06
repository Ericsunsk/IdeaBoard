import { describe, expect, it } from 'vitest'
import type { AdminTaskMonitorListItemModel } from './taskMonitorResource'
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

  it('does not infer an API call from mutable result metadata', () => {
    const task = makeTask({
      status: adminTaskStatuses.completed,
      result: JSON.stringify({
        platform: 'google',
        image_url: '/uploads/tasks/1/task/output.png',
      }),
    })

    expect(getTaskApiCall(task)).toBeNull()
    expect(getTaskResultImageUrl(task)).toBe('/uploads/tasks/1/task/output.png')
  })

  it('uses the latest immutable execution API Type over legacy result metadata', () => {
    const task = makeTask({
      latest_execution: {
        account_id: 1,
        provider_endpoint_id: 2,
        provider_endpoint_name: 'Provider A',
        resolved_model: 'model-a',
        api_type: 'openai-compatible',
      },
      result: JSON.stringify({
        platform: 'google',
      }),
    })

    expect(getTaskApiCall(task)).toEqual({ platform: 'openai-compatible' })
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

  it('summarizes provider parsed-output failures for monitor detail views', () => {
    const result = JSON.stringify({
      status: 'failed',
      parsed_output_status: 'invalid',
      error: '图案生成输出不符合 workflow contract',
      failure_stage: 'provider',
      failure_category: 'provider_error',
      raw_provider_output_preview: '{"data":[]}',
      parsed_output: {
        kind: 'image-results',
        image_count: 0,
      },
    })
    const summary = JSON.parse(formatTaskResultSummary(result))

    expect(summary).toMatchObject({
      status: 'failed',
      parsed_output_status: 'invalid',
      error: '图案生成输出不符合 workflow contract',
      failure_stage: 'provider',
      failure_category: 'provider_error',
      raw_provider_output_preview: '{"data":[]}',
      parsed_output: {
        kind: 'image-results',
        image_count: 0,
      },
    })
    expect(getTaskErrorReason(makeTask({ status: adminTaskStatuses.failed, result }))).toBe(
      '图案生成输出不符合 workflow contract',
    )
  })
})

function makeTask(
  overrides: Partial<AdminTaskMonitorListItemModel> = {},
): AdminTaskMonitorListItemModel {
  return {
    task_id: 'task_1',
    user_public_id: '00000000-0000-0000-0000-000000000001',
    user_display_name: 'User 1',
    prompt: 'Prompt',
    priority: 2,
    status: adminTaskStatuses.completed,
    error_msg: '',
    result: '',
    created_at: '2026-07-06T00:00:00Z',
    completed_at: null,
    ...overrides,
  }
}
