import { beforeEach, describe, expect, it, vi } from 'vitest'
import { insufficientCreditsPreflightError } from '../task-runner/canvasBillingPreflight'
import { subscribeCanvasTaskReadiness } from '../billing/taskReadinessEvents'
import { assertTaskRequestReady, submitTaskRequest } from './canvasTaskSubmissionAdapter'
import type {
  CanvasTaskRequestData,
  SubmitTaskReadinessData,
} from '../../../core/execution/canvasTaskRequest'

const mocks = vi.hoisted(() => ({
  fetchReadiness: vi.fn(),
  submit: vi.fn(),
}))

vi.mock('./canvasWorkspaceTaskAdapter', () => ({
  fetchCanvasWorkspaceTaskReadiness: mocks.fetchReadiness,
  submitCanvasWorkspaceTask: mocks.submit,
}))

describe('task readiness billing updates', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('publishes the fresh server quote before blocking an insufficient submission', async () => {
    const readiness: SubmitTaskReadinessData = {
      ready: false,
      message: 'Idea不足',
      service: 'generation',
      billing: {
        sku: 'ai-image:generation',
        price_version_id: 2,
        price_version: 2,
        unit_credits: 6,
        quantity: 4,
        total_credits: 24,
        quoted_at: '2026-07-12T00:00:00.000Z',
        available_credits: 3,
        reserved_credits: 0,
        sufficient: false,
      },
    }
    mocks.fetchReadiness.mockResolvedValue(readiness)
    const events: string[] = []
    const listener = vi.fn(() => events.push('published'))
    const unsubscribe = subscribeCanvasTaskReadiness(listener)

    await expect(
      assertTaskRequestReady({
        prompt: 'final prompt',
        service: 'generation',
        client_action_id: 'canvas:action-1',
        params: { output_count: 4 },
      }).catch((error: unknown) => {
        events.push('rejected')
        throw error
      }),
    ).rejects.toThrow('You need more Ideas to run this action.')

    expect(events).toEqual(['published', 'rejected'])
    expect(listener).toHaveBeenCalledWith(readiness)
    unsubscribe()
  })

  it.each([
    { message: 'Board is read-only', expected: 'Board is read-only' },
    { message: undefined, expected: 'Could not submit the task. Please try again.' },
  ])(
    'publishes a not-ready Quote before rejecting with $expected',
    async ({ message, expected }) => {
      const readiness: SubmitTaskReadinessData = {
        ready: false,
        service: 'generation',
        ...(message === undefined ? {} : { message }),
      }
      mocks.fetchReadiness.mockResolvedValue(readiness)
      const listener = vi.fn()
      const unsubscribe = subscribeCanvasTaskReadiness(listener)
      try {
        await expect(assertTaskRequestReady({ prompt: 'a cat' })).rejects.toThrow(expected)
        expect(listener).toHaveBeenCalledWith(readiness)
        expect(mocks.submit).not.toHaveBeenCalled()
      } finally {
        unsubscribe()
      }
    },
  )

  it('keeps an explicit free quote ready without requiring wallet coverage', async () => {
    const readiness: SubmitTaskReadinessData = {
      ready: true,
      service: 'generation',
      billing: {
        sku: 'ai-image:generation.test',
        price_version_id: 2,
        price_version: 2,
        unit_credits: 0,
        quantity: 1,
        total_credits: 0,
        quoted_at: '2026-07-12T00:00:00.000Z',
        available_credits: 0,
        reserved_credits: 0,
        sufficient: true,
      },
    }
    mocks.fetchReadiness.mockResolvedValue(readiness)

    await expect(
      assertTaskRequestReady({
        prompt: 'free prompt',
        service: 'generation',
        client_action_id: 'canvas:free-action',
        params: { output_count: 4 },
      }),
    ).resolves.toBeUndefined()
  })

  it('preserves the insufficient-balance classification already produced by Quote', async () => {
    const error = insufficientCreditsPreflightError()
    mocks.fetchReadiness.mockRejectedValueOnce(error)
    await expect(assertTaskRequestReady({ prompt: 'a cat' })).rejects.toBe(error)
  })

  it('classifies confirmed browser connection failures separately from server quote failures', async () => {
    mocks.fetchReadiness.mockRejectedValueOnce(new Error('fetch failed'))

    await expect(
      assertTaskRequestReady({
        prompt: 'network prompt',
        service: 'generation',
        params: { output_count: 1 },
      }),
    ).rejects.toMatchObject({ kind: 'network', message: 'The request could not be completed.' })

    mocks.fetchReadiness.mockRejectedValueOnce(
      Object.assign(new Error('pricing unavailable'), {
        status: 503,
        code: 'billing_quote_unavailable',
      }),
    )

    await expect(
      assertTaskRequestReady({
        prompt: 'quote prompt',
        service: 'generation',
        params: { output_count: 1 },
      }),
    ).rejects.toMatchObject({
      kind: 'pricing-unavailable',
      message: 'Pricing is temporarily unavailable. Please try again shortly.',
    })
  })

  it('blocks an unavailable resolution with the clear pool copy', async () => {
    mocks.fetchReadiness.mockResolvedValue({
      ready: false,
      service: 'generation',
      error_code: 'service_resolution_unavailable',
      message: 'This resolution is unavailable right now.',
    } satisfies SubmitTaskReadinessData)

    await expect(
      assertTaskRequestReady({
        prompt: 'a cat',
        service: 'generation',
        params: { output_count: 1, output_resolution: '2k' },
      }),
    ).rejects.toThrow('This resolution is unavailable right now.')
  })
})

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
