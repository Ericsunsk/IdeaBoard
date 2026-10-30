import { beforeEach, describe, expect, it, vi } from 'vitest'
import { submitTaskRequest } from './canvasTaskSubmissionAdapter'
import type { CanvasTaskRequestData } from '../../../core/execution/canvasTaskRequest'

const mocks = vi.hoisted(() => ({ submit: vi.fn() }))

vi.mock('./canvasWorkspaceTaskAdapter', () => ({
  submitCanvasWorkspaceTask: mocks.submit,
}))

describe('task acceptance submission', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.submit.mockResolvedValue({ taskId: 'task-upscale', workspaceId: 'ws_test123' })
  })

  it('submits a prepared task request and returns the task id', async () => {
    const request: CanvasTaskRequestData = {
      prompt: '',
      service: 'upscale',
      media_asset_ids: ['asset_aaa1'],
      client_action_id: 'canvas:action-1',
      board_id: 'brd_board1',
    }

    await expect(submitTaskRequest(request)).resolves.toEqual({ taskId: 'task-upscale' })
    expect(mocks.submit).toHaveBeenCalledWith(request)
  })

  it('lets transport id errors propagate without local sanitization', async () => {
    // 空 taskId 不再本地设防：传输层 `toWorkspaceTask` 的 opaque 断言抛错，
    // 此处只验证透传 + 计费映射（不再吞错改写 fallback）。
    mocks.submit.mockRejectedValueOnce(
      Object.assign(new Error('Invalid task id'), { status: 400, code: 'bad_request' }),
    )

    await expect(submitTaskRequest({ prompt: '' })).rejects.toThrow('Invalid task id')
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

  it('maps a submit-time empty pool (422) to the resolution toast copy', async () => {
    mocks.submit.mockRejectedValueOnce(
      Object.assign(new Error('no eligible model'), {
        status: 422,
        code: 'service_resolution_unavailable',
      }),
    )

    await expect(
      submitTaskRequest({ prompt: 'a cat', service: 'generation' }),
    ).rejects.toMatchObject({
      kind: 'resolution-unavailable',
      message: 'This resolution is unavailable right now.',
    })
    expect(mocks.submit).toHaveBeenCalledTimes(1)
  })
})
