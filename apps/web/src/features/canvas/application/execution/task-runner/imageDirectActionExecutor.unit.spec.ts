import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { SubmitTaskData } from '../../../domain/execution-contracts/canvasTaskRequest'
import { TaskResultPendingError } from './taskResultErrors'
import { submitTaskRequest, waitExistingOrLiveTaskImageResults } from './imageDirectActionExecutor'

const mocks = vi.hoisted(() => ({
  submit: vi.fn(),
  wait: vi.fn(),
}))

vi.mock('../workspace/canvasWorkspaceTaskAdapter', () => ({
  submitCanvasWorkspaceTask: mocks.submit,
  waitCanvasWorkspaceTaskImageResults: mocks.wait,
}))

describe('imageDirectActionExecutor', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.submit.mockResolvedValue({ taskId: 'task-upscale', workspaceId: 'ws_test123' })
  })

  it('submits a prepared task request and returns the task id', async () => {
    const request: SubmitTaskData = {
      prompt: '',
      feature: 'upscale',
      media_asset_ids: ['asset_aaa1'],
    }

    await expect(submitTaskRequest(request)).resolves.toEqual({ taskId: 'task-upscale' })
    expect(mocks.submit).toHaveBeenCalledWith(request)
  })

  it('sanitizes submit failures with a fallback message', async () => {
    mocks.submit.mockResolvedValueOnce({ taskId: '', workspaceId: 'ws_test123' })

    await expect(submitTaskRequest({ prompt: '' }, '提交失败')).rejects.toThrow('提交失败')
  })

  it('classifies only the workspace insufficient-credits machine code as wallet insufficiency', async () => {
    mocks.submit.mockRejectedValueOnce(
      Object.assign(new Error('You need more Ideas to run this action.'), {
        status: 402,
        code: 'task_insufficient_credits',
      }),
    )

    await expect(submitTaskRequest({ prompt: '' })).rejects.toMatchObject({
      kind: 'insufficient-credits',
      message: 'You need more Ideas to run this action.',
    })

    mocks.submit.mockRejectedValueOnce(
      Object.assign(new Error('insufficient_quota'), { status: 402, code: 402 }),
    )

    await expect(submitTaskRequest({ prompt: '' })).rejects.not.toMatchObject({
      kind: 'insufficient-credits',
    })
  })

  it('maps a confirmed browser connection failure during submit to the network copy', async () => {
    mocks.submit.mockRejectedValueOnce(new Error('fetch failed'))

    await expect(submitTaskRequest({ prompt: '' })).rejects.toMatchObject({
      kind: 'network',
      message: 'The request could not be completed.',
    })
  })

  it('maps an authoritative submit pricing race to the pricing-unavailable contract', async () => {
    mocks.submit.mockRejectedValueOnce(
      Object.assign(new Error('暂时无法获取报价，请重试'), {
        status: 503,
        code: 'billing_quote_unavailable',
      }),
    )

    await expect(submitTaskRequest({ prompt: '' })).rejects.toMatchObject({
      kind: 'pricing-unavailable',
      message: 'Pricing is temporarily unavailable. Please try again shortly.',
    })
  })

  it('delegates result waiting to the workspace waiter', async () => {
    mocks.wait.mockResolvedValueOnce([{ url: 'https://cdn.example/out.png', resultIndex: 0 }])

    await expect(waitExistingOrLiveTaskImageResults('task_aaa111')).resolves.toEqual([
      { url: 'https://cdn.example/out.png', resultIndex: 0 },
    ])
    expect(mocks.wait).toHaveBeenCalledWith('task_aaa111')
  })

  it('surfaces waiter timeouts as pending', async () => {
    mocks.wait.mockRejectedValueOnce(new TaskResultPendingError('timeout'))

    await expect(waitExistingOrLiveTaskImageResults('task_aaa111')).rejects.toThrow(
      TaskResultPendingError,
    )
  })
})
