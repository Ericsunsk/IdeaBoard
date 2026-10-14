import { describe, expect, it } from 'vitest'
import type { AdminMonitorListItemModel } from '../resource'
import {
  formatTaskDuration,
  formatTaskResultDebug,
  formatTaskResultSummary,
  getTaskDuration,
  getTaskErrorReason,
  getTaskResultImageUrl,
  getTaskSanitizedError,
} from './taskResultPresentation'
import { adminTaskStatuses } from './status'

describe('task result presentation', () => {
  it('summarizes result payloads without exposing sensitive debug fields', () => {
    const summary = JSON.parse(
      formatTaskResultSummary(
        JSON.stringify({
          status: 'completed',
          platform: 'openai',
          images: [
            '/api/v1/workspaces/ws_test123/media-assets/asset_a/content',
            '/api/v1/workspaces/ws_test123/media-assets/asset_b/content',
          ],
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

  it('reads the result image from result metadata', () => {
    const contentUrl = '/api/v1/workspaces/ws_test123/media-assets/asset_123/content'
    const task = makeTask({
      status: adminTaskStatuses.completed,
      result: JSON.stringify({
        platform: 'google',
        image_url: contentUrl,
      }),
    })

    expect(getTaskResultImageUrl(task)).toBe(contentUrl)
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

function makeTask(overrides: Partial<AdminMonitorListItemModel> = {}): AdminMonitorListItemModel {
  return {
    task_id: 'task_1',
    user_public_id: '00000000-0000-0000-0000-000000000001',
    user_display_name: 'User 1',
    prompt: 'Prompt',
    service: 'render',
    credits: 0,
    priority: 2,
    status: adminTaskStatuses.completed,
    error_msg: '',
    result: '',
    created_at: '2026-07-06T00:00:00Z',
    completed_at: null,
    ...overrides,
  } as AdminMonitorListItemModel
}
