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
    mocks.submit.mockResolvedValue({ code: 0, data: { task_id: 'task-hd' } })
  })

  it('submits a prepared task request and returns the task id', async () => {
    const request: SubmitTaskData = {
      prompt: 'generate',
      feature: 'pattern-gen',
    }

    await expect(submitTaskRequest(request)).resolves.toEqual({ taskId: 'task-hd' })
    expect(mocks.submit).toHaveBeenCalledWith(request)
  })

  it('sanitizes submit failures with a fallback message', async () => {
    mocks.submit.mockResolvedValueOnce({ code: 400, msg: '', data: {} })

    await expect(submitTaskRequest({ prompt: 'generate' }, '提交失败')).rejects.toThrow('提交失败')
  })

  it('returns canonical image results from completed task fast path', async () => {
    mocks.get.mockResolvedValueOnce({
      code: 0,
      data: {
        task: {
          status: 'completed',
          outputs: [
            {
              role: 'image',
              url: ' https://cdn.example.com/done.png ',
              width: 1000,
              height: 750,
              mime_type: ' image/png ',
            },
          ],
        },
      },
    })

    await expect(waitExistingOrLiveTaskImageResults('task-done')).resolves.toEqual([
      {
        url: 'https://cdn.example.com/done.png',
        width: 1000,
        height: 750,
        mimeType: 'image/png',
      },
    ])
  })

  it('keeps storage materialization failures pending on the existing task fast path', async () => {
    mocks.get.mockResolvedValueOnce({
      code: 0,
      data: {
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
      },
    })

    await expect(waitExistingOrLiveTaskImageResults('task-storage')).rejects.toThrow(
      TaskResultPendingError,
    )
  })

})
