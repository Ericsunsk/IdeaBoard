import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { SubmitTaskData } from '../../../../shared/api/task'
import { TaskResultPendingError } from '../../../../shared/api/taskEvents'
import {
  submitTaskRequest,
  waitExistingOrLiveTaskImageResults,
} from './flowImageDirectActionExecutor'

const mocks = vi.hoisted(() => ({
  get: vi.fn(),
  submit: vi.fn(),
}))

vi.mock('../../../../shared/api/task', () => ({
  taskApi: {
    get: mocks.get,
    submit: mocks.submit,
  },
}))

describe('flowImageDirectActionExecutor', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.get.mockReset()
    mocks.submit.mockResolvedValue({ task_id: 'task-upscale' })
  })

  it('submits a prepared task request and returns the task id', async () => {
    const request: SubmitTaskData = {
      prompt: 'generate',
      feature: 'generation',
    }

    await expect(submitTaskRequest(request)).resolves.toEqual({ taskId: 'task-upscale' })
    expect(mocks.submit).toHaveBeenCalledWith(request)
  })

  it('sanitizes submit failures with a fallback message', async () => {
    mocks.submit.mockResolvedValueOnce({})

    await expect(submitTaskRequest({ prompt: 'generate' }, '提交失败')).rejects.toThrow('提交失败')
  })

  it('classifies only the Task Submit machine code as user wallet insufficiency', async () => {
    mocks.submit.mockRejectedValueOnce(
      Object.assign(new Error('You need more Ideas to run this action.'), {
        status: 402,
        code: 'insufficient_credits',
      }),
    )

    await expect(submitTaskRequest({ prompt: 'generate' })).rejects.toMatchObject({
      kind: 'insufficient-credits',
      message: 'You need more Ideas to run this action.',
    })

    mocks.submit.mockRejectedValueOnce(
      Object.assign(new Error('insufficient_quota'), { status: 402, code: 402 }),
    )

    await expect(submitTaskRequest({ prompt: 'generate' })).rejects.not.toMatchObject({
      kind: 'insufficient-credits',
    })
  })

  it('maps a confirmed browser connection failure during submit to the network copy', async () => {
    mocks.submit.mockRejectedValueOnce(new Error('fetch failed'))

    await expect(submitTaskRequest({ prompt: 'generate' })).rejects.toMatchObject({
      kind: 'network',
      message: 'The request could not be completed.',
    })
  })

  it('maps an authoritative submit pricing race to the pricing-unavailable contract', async () => {
    mocks.submit.mockRejectedValueOnce(
      Object.assign(new Error('暂时无法获取报价，请重试'), {
        status: 503,
        code: 'pricing_unavailable',
      }),
    )

    await expect(submitTaskRequest({ prompt: 'generate' })).rejects.toMatchObject({
      kind: 'pricing-unavailable',
      message: 'Pricing is temporarily unavailable. Please try again shortly.',
    })
  })

  it('returns canonical image results from completed task fast path', async () => {
    mocks.get.mockResolvedValueOnce({
      task: {
        status: 'completed',
        outputs: [
          {
            role: 'image',
            url: '',
            asset_id: 'asset_done',
            width: 1000,
            height: 750,
            mime_type: ' image/png ',
          },
        ],
      },
    })

    await expect(waitExistingOrLiveTaskImageResults('task-done')).resolves.toEqual([
      {
        url: '/api/user/media-assets/asset_done/derivatives/preview/webp/content',
        assetId: 'asset_done',
        width: 1000,
        height: 750,
        mimeType: 'image/png',
      },
    ])
  })

  it('keeps storage materialization failures pending on the existing task fast path', async () => {
    mocks.get.mockResolvedValueOnce({
      task: {
        status: 'failed',
        error_msg: '图片结果保存失败，请检查存储配置',
        execution_metrics: {
          task_id: 'task-storage',
          current_stage: 'failed',
          bottleneck_source: 'storage',
          failure_stage: 'finalizing',
          failure_category: 'storage_error',
          queue_wait_ms: 0,
          worker_overhead_ms: 0,
          provider_latency_ms: 100,
          finalize_latency_ms: 20,
          total_latency_ms: 120,
          created_at: '2026-01-01T00:00:00.000Z',
          updated_at: '2026-01-01T00:00:00.000Z',
        },
      },
    })

    await expect(waitExistingOrLiveTaskImageResults('task-storage')).rejects.toThrow(
      TaskResultPendingError,
    )
  })
})
