// Canvas workspace task adapter spec (W2 stream 1).
//
// Covers the explicit SubmitTaskData -> v1 semantic mapping: submit input
// mapping + fail-closed rules, quote -> readiness mapping, output-ref image
// extraction, terminal settlement, and cancel workspace resolution. The
// central `shared/api/workspaceTasks` resource is mocked (its own contract
// spec covers `/api/v1` over MSW); the workspace selection store is stubbed.
import { beforeEach, describe, expect, it, vi } from 'vitest'
import {
  CANVAS_WORKSPACE_REQUIRED_MESSAGE,
  cancelCanvasWorkspaceTask,
  CanvasWorkspaceTaskUnmappableError,
  CanvasWorkspaceUnavailableError,
  extractWorkspaceTaskImageResults,
  fetchCanvasWorkspaceTaskReadiness,
  fetchCanvasWorkspaceTaskReadinessMatrix,
  registerCanvasWorkspaceTask,
  resetCanvasWorkspaceTaskRegistryForTests,
  submitCanvasWorkspaceTask,
  toSubmitTaskReadiness,
  toWorkspaceCreateInput,
  toWorkspaceQuoteInput,
  waitCanvasWorkspaceTaskImageResults,
} from './canvasWorkspaceTaskAdapter'

const mocks = vi.hoisted(() => ({
  createWorkspaceTask: vi.fn(),
  quoteWorkspaceTask: vi.fn(),
  getWorkspaceTask: vi.fn(),
  waitWorkspaceTaskTerminal: vi.fn(),
  cancelWorkspaceTask: vi.fn(),
  selectedId: { value: 'ws_test123' as string | null },
}))

vi.mock('../../../../../shared/api/workspaceTasks', () => ({
  cancelWorkspaceTask: mocks.cancelWorkspaceTask,
  createWorkspaceTask: mocks.createWorkspaceTask,
  getWorkspaceTask: mocks.getWorkspaceTask,
  quoteWorkspaceTask: mocks.quoteWorkspaceTask,
  waitWorkspaceTaskTerminal: mocks.waitWorkspaceTaskTerminal,
}))

vi.mock('../../../../workspace', () => ({
  getWorkspaceSelectionStore: () => ({
    selectedId: mocks.selectedId,
    getSelectedId: () => mocks.selectedId.value,
  }),
}))

const WORKSPACE_ID = 'ws_test123'

function taskItem(overrides: Record<string, unknown> = {}) {
  return {
    id: 'task_aaa111',
    workspace_id: WORKSPACE_ID,
    service: 'upscale',
    status: 'completed',
    retry_count: 0,
    billing_snapshot: {
      sku: 'ai-image:upscale',
      display_name: 'Upscale',
      price_version_id: 2,
      price_version: 2,
      output_resolution: 'default',
      unit_credits: 6,
      quantity: 1,
      total_credits: 6,
      quoted_at: '2026-09-25T00:00:00.000Z',
      settlement_state: 'held',
    },
    created_at: '2026-09-25T00:00:00.000Z',
    updated_at: '2026-09-25T00:00:00.000Z',
    ...overrides,
  }
}

function expectUnmappable(run: () => unknown, reason: string): CanvasWorkspaceTaskUnmappableError {
  try {
    run()
  } catch (error) {
    expect(error).toBeInstanceOf(CanvasWorkspaceTaskUnmappableError)
    expect((error as CanvasWorkspaceTaskUnmappableError).reason).toBe(reason)
    return error as CanvasWorkspaceTaskUnmappableError
  }
  throw new Error(`expected unmappable (${reason})`)
}

describe('toWorkspaceCreateInput', () => {
  it('maps service, opaque media refs, params, quantity, and resolution', () => {
    expect(
      toWorkspaceCreateInput({
        prompt: '   ',
        service: 'upscale',
        client_action_id: 'canvas:act-1',
        priority: 1,
        media_asset_ids: ['asset_aaa1', 'asset_aaa1', ' asset_bbb2 '],
        params: {
          output_count: 2,
          output_resolution: '1k',
          upscale: { scale: 2 },
        },
      }),
    ).toEqual({
      service: 'upscale',
      media_asset_ids: ['asset_aaa1', 'asset_bbb2'],
      params: {
        output_count: 2,
        output_resolution: '1k',
        upscale: { scale: 2 },
      },
      quantity: 2,
      resolution: '1k',
    })
  })

  it('maps non-empty prompts onto data.prompt for prompt-carrying services', () => {
    expect(toWorkspaceCreateInput({ prompt: '  a cat  ', service: 'generation' }).prompt).toBe(
      'a cat',
    )
    expect(
      toWorkspaceCreateInput({
        prompt: 'redraw the area',
        service: 'inpaint',
        media_asset_ids: ['asset_aaa1'],
        params: { mask_asset_id: 'asset_mask1', use_mask: true },
      }),
    ).toMatchObject({
      service: 'inpaint',
      prompt: 'redraw the area',
      media_asset_ids: ['asset_aaa1'],
      params: { mask_asset_id: 'asset_mask1', use_mask: true },
    })
  })

  it('mirrors the backend prompt policy per service (required/optional/ignored)', () => {
    // Required (generation, inpaint) and optional (render, variations,
    // fusion, outpaint) carry user text; blank prompts ride as absent so
    // the server 422 stays authoritative for required-policy empties.
    for (const service of ['generation', 'inpaint', 'render', 'variations', 'fusion', 'outpaint']) {
      expect(toWorkspaceCreateInput({ prompt: 'keep me', service }).prompt).toBe('keep me')
      expect(toWorkspaceCreateInput({ prompt: '   ', service })).not.toHaveProperty('prompt')
    }
    // Ignored (cutout, upscale) never send user text, even when non-blank.
    for (const service of ['cutout', 'upscale']) {
      expect(toWorkspaceCreateInput({ prompt: 'drop me', service })).not.toHaveProperty('prompt')
    }
  })

  it('fails closed on over-8KB-byte prompts with byte (not char) length', () => {
    expect(
      toWorkspaceCreateInput({ prompt: 'a'.repeat(8192), service: 'generation' }).prompt,
    ).toHaveLength(8192)
    // Multibyte text counts bytes: 5000 e-acutes are 10000 bytes.
    expectUnmappable(
      () => toWorkspaceCreateInput({ prompt: 'é'.repeat(5000), service: 'generation' }),
      'prompt',
    )
    expectUnmappable(
      () => toWorkspaceCreateInput({ prompt: `x${'y'.repeat(8192)}`, service: 'render' }),
      'prompt',
    )
  })

  it('fails closed on unknown services', () => {
    expectUnmappable(() => toWorkspaceCreateInput({ prompt: '', service: 'teleport' }), 'service')
  })

  it('fails closed on image URLs and numeric asset ids', () => {
    expectUnmappable(
      () =>
        toWorkspaceCreateInput({
          prompt: '',
          service: 'generation',
          image_urls: ['https://cdn.example/a.png'],
        }),
      'media-urls',
    )
    expectUnmappable(
      () => toWorkspaceCreateInput({ prompt: '', service: 'generation', image_asset_ids: [12] }),
      'media-ids',
    )
    expectUnmappable(
      () =>
        toWorkspaceCreateInput({
          prompt: '',
          service: 'generation',
          reference_image_asset_ids: [7],
        }),
      'media-ids',
    )
  })

  it('fails closed on legacy mask data-URLs with mask-specific guidance', () => {
    expectUnmappable(
      () =>
        toWorkspaceCreateInput({
          prompt: '',
          service: 'inpaint',
          params: { mask_image_url: 'data:image/png;base64,AAA', use_mask: true },
        }),
      'media-urls',
    )
    try {
      toWorkspaceCreateInput({
        prompt: '',
        service: 'inpaint',
        params: { mask_image_url: 'data:image/png;base64,AAA', use_mask: true },
      })
      expect.unreachable()
    } catch (error) {
      expect(error).toBeInstanceOf(CanvasWorkspaceTaskUnmappableError)
      expect((error as Error).message).toContain('workspace media assets')
    }
  })

  it('maps mask_asset_id opaque refs and fails closed on mask URLs/shapes', () => {
    expect(
      toWorkspaceCreateInput({
        prompt: 'redraw',
        service: 'inpaint',
        params: { mask_asset_id: ' asset_mask1 ', use_mask: true },
      }).params,
    ).toMatchObject({ mask_asset_id: 'asset_mask1', use_mask: true })
    // Data-URL masks cannot ride params: upload via the session flow first.
    expectUnmappable(
      () =>
        toWorkspaceCreateInput({
          prompt: 'redraw',
          service: 'inpaint',
          params: { mask_asset_id: 'data:image/png;base64,AAA' },
        }),
      'media-urls',
    )
    // Non-asset shapes fail closed as media-ids.
    expectUnmappable(
      () =>
        toWorkspaceCreateInput({
          prompt: 'redraw',
          service: 'inpaint',
          params: { mask_asset_id: 'not-an-id' },
        }),
      'media-ids',
    )
    expectUnmappable(
      () =>
        toWorkspaceCreateInput({
          prompt: 'redraw',
          service: 'inpaint',
          params: { mask_asset_id: '42' },
        }),
      'media-ids',
    )
    // Mask policy (inpaint-only) stays server-side: a well-shaped mask id
    // on another service threads through instead of failing closed here.
    expect(
      toWorkspaceCreateInput({
        prompt: '',
        service: 'render',
        params: { mask_asset_id: 'asset_mask1' },
      }).params,
    ).toMatchObject({ mask_asset_id: 'asset_mask1' })
  })

  it('fails closed on unknown params and storage URLs instead of a backend 422', () => {
    expectUnmappable(
      () => toWorkspaceCreateInput({ prompt: '', service: 'render', params: { prompt: 'hi' } }),
      'params',
    )
    expectUnmappable(
      () =>
        toWorkspaceCreateInput({
          prompt: '',
          service: 'render',
          params: { fusion: { mode: 'weighted_reference', note: 's3://bucket/x' } },
        }),
      'params',
    )
  })

  it('rejects malformed opaque asset ids', () => {
    expectUnmappable(
      () =>
        toWorkspaceCreateInput({
          prompt: '',
          service: 'render',
          media_asset_ids: ['not-an-id'],
        }),
      'media-ids',
    )
  })
})

describe('quote mapping', () => {
  it('quotes price-only inputs, ignoring prompt and media', () => {
    expect(
      toWorkspaceQuoteInput({
        prompt: 'a cat (never sent)',
        service: 'generation',
        image_urls: ['https://cdn.example/a.png'],
        media_asset_ids: ['asset_aaa1'],
        params: { output_count: 3, output_resolution: '1k' },
      }),
    ).toEqual({ service: 'generation', quantity: 3, resolution: '1k' })
  })

  it('maps a snapshot to readiness with inferred sufficiency', () => {
    expect(
      toSubmitTaskReadiness(
        {
          unit_credits: 6,
          total_credits: 24,
          price_version_id: 2,
          price_version: 2,
          quantity: 4,
        },
        'generation',
      ),
    ).toEqual({
      ready: true,
      service: 'generation',
      billing: {
        sufficient: true,
        unit_credits: 6,
        total_credits: 24,
        available_credits: 0,
        reserved_credits: 0,
        price_version_id: 2,
        price_version: 2,
        quantity: 4,
      },
    })
  })
})

describe('canvas workspace task transport', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.selectedId.value = WORKSPACE_ID
    resetCanvasWorkspaceTaskRegistryForTests()
  })

  it('submits prompts and mask refs in the v1 body with a header-only Idempotency-Key', async () => {
    mocks.createWorkspaceTask.mockResolvedValue(taskItem({ status: 'pending' }))

    await expect(
      submitCanvasWorkspaceTask({
        prompt: 'a cat',
        service: 'generation',
        client_action_id: 'canvas:act-2',
        params: { output_count: 1, output_resolution: '1k' },
      }),
    ).resolves.toEqual({ taskId: 'task_aaa111', workspaceId: WORKSPACE_ID })

    expect(mocks.createWorkspaceTask).toHaveBeenCalledWith(
      WORKSPACE_ID,
      {
        service: 'generation',
        prompt: 'a cat',
        params: { output_count: 1, output_resolution: '1k' },
        quantity: 1,
        resolution: '1k',
      },
      { idempotencyKey: 'canvas:act-2' },
    )
  })

  it('keeps the idempotency key prompt-independent so server 409 stays observable', async () => {
    // The prompt is covered by the server idempotency hash, not the client
    // key: the same client_action_id with a different prompt reuses the key
    // (server renders 409 task_idempotency_conflict, never a silent replay).
    // Client-side only the body difference is observable; assert both halves.
    mocks.createWorkspaceTask.mockResolvedValue(taskItem({ status: 'pending' }))

    await submitCanvasWorkspaceTask({
      prompt: 'a cat',
      service: 'generation',
      client_action_id: 'canvas:act-3',
    })
    await submitCanvasWorkspaceTask({
      prompt: 'a dog',
      service: 'generation',
      client_action_id: 'canvas:act-3',
    })

    const calls = mocks.createWorkspaceTask.mock.calls
    expect(calls).toHaveLength(2)
    expect(calls[0]?.[2]).toEqual({ idempotencyKey: 'canvas:act-3' })
    expect(calls[1]?.[2]).toEqual({ idempotencyKey: 'canvas:act-3' })
    expect((calls[0]?.[1] as { prompt?: string }).prompt).toBe('a cat')
    expect((calls[1]?.[1] as { prompt?: string }).prompt).toBe('a dog')
  })

  it('submits with the client action id as header-only Idempotency-Key', async () => {
    mocks.createWorkspaceTask.mockResolvedValue(taskItem({ status: 'pending' }))

    await expect(
      submitCanvasWorkspaceTask({
        prompt: '',
        service: 'upscale',
        client_action_id: 'canvas:act-1',
        media_asset_ids: ['asset_aaa1'],
        params: { upscale: { scale: 2 } },
      }),
    ).resolves.toEqual({ taskId: 'task_aaa111', workspaceId: WORKSPACE_ID })

    expect(mocks.createWorkspaceTask).toHaveBeenCalledWith(
      WORKSPACE_ID,
      {
        service: 'upscale',
        media_asset_ids: ['asset_aaa1'],
        params: { upscale: { scale: 2 } },
      },
      { idempotencyKey: 'canvas:act-1' },
    )
  })

  it('refuses submit without a workspace selection', async () => {
    mocks.selectedId.value = null

    await expect(
      submitCanvasWorkspaceTask({ prompt: '', service: 'upscale' }),
    ).rejects.toMatchObject({
      name: 'CanvasWorkspaceUnavailableError',
      message: CANVAS_WORKSPACE_REQUIRED_MESSAGE,
    })
    expect(mocks.createWorkspaceTask).not.toHaveBeenCalled()
  })

  it('maps a 402 quote to insufficient-credits and a 422 quote to not-ready data', async () => {
    mocks.quoteWorkspaceTask.mockRejectedValueOnce(
      Object.assign(new Error('Idea不足'), { status: 402, code: 'task_insufficient_credits' }),
    )
    await expect(
      fetchCanvasWorkspaceTaskReadiness({ prompt: '', service: 'generation' }),
    ).rejects.toMatchObject({ name: 'CanvasBillingPreflightError', kind: 'insufficient-credits' })

    mocks.quoteWorkspaceTask.mockRejectedValueOnce(
      Object.assign(new Error('bad kind'), { status: 422, code: 'validation_failed' }),
    )
    await expect(
      fetchCanvasWorkspaceTaskReadiness({ prompt: '', service: 'generation' }),
    ).resolves.toMatchObject({ ready: false, service: 'generation' })
  })

  it('preserves the unavailable resolution code from quote readiness', async () => {
    mocks.quoteWorkspaceTask.mockRejectedValueOnce(
      Object.assign(new Error('unavailable'), {
        status: 422,
        code: 'service_resolution_unavailable',
      }),
    )
    await expect(
      fetchCanvasWorkspaceTaskReadiness({ prompt: '', service: 'generation' }),
    ).resolves.toMatchObject({
      ready: false,
      service: 'generation',
      error_code: 'service_resolution_unavailable',
      message: 'This resolution is unavailable right now.',
    })
  })

  it('threads an optional caller signal to the quote transport', async () => {
    mocks.quoteWorkspaceTask.mockResolvedValue({
      unit_credits: 6,
      total_credits: 6,
      price_version_id: 2,
      price_version: 2,
      quantity: 1,
    })
    const controller = new AbortController()

    await fetchCanvasWorkspaceTaskReadiness(
      { prompt: '', service: 'generation' },
      { signal: controller.signal },
    )

    expect(mocks.quoteWorkspaceTask).toHaveBeenCalledWith(
      WORKSPACE_ID,
      expect.objectContaining({ service: 'generation' }),
      { signal: controller.signal },
    )
  })

  it('fans matrix readiness out to one quote per request', async () => {
    mocks.quoteWorkspaceTask
      .mockResolvedValueOnce({
        unit_credits: 6,
        total_credits: 6,
        price_version_id: 2,
        price_version: 2,
        quantity: 1,
      })
      .mockResolvedValueOnce({
        unit_credits: 0,
        total_credits: 0,
        price_version_id: 2,
        price_version: 2,
        quantity: 1,
      })

    const items = await fetchCanvasWorkspaceTaskReadinessMatrix([
      { prompt: '', service: 'generation', params: { output_count: 1 } },
      { prompt: '', service: 'cutout', params: { output_count: 1 } },
    ])

    expect(items).toHaveLength(2)
    expect(items[0]).toMatchObject({ ready: true, service: 'generation' })
    expect(items[1]).toMatchObject({ ready: true, service: 'cutout' })
    expect(mocks.quoteWorkspaceTask).toHaveBeenCalledTimes(2)
    await expect(fetchCanvasWorkspaceTaskReadinessMatrix([])).resolves.toEqual([])
  })
})

describe('extractWorkspaceTaskImageResults', () => {
  it('resolves asset refs through the media browser URL', () => {
    expect(
      extractWorkspaceTaskImageResults(WORKSPACE_ID, [
        { kind: 'asset', asset_id: 'asset_aaa1', role: 'image', result_index: 0 },
      ]),
    ).toEqual([
      {
        url: '/api/v1/workspaces/ws_test123/media-assets/asset_aaa1/derivatives/preview',
        assetId: 'asset_aaa1',
        resultIndex: 0,
      },
    ])
  })

  it('accepts blank roles and resolves remote refs direct', () => {
    expect(
      extractWorkspaceTaskImageResults(WORKSPACE_ID, [
        {
          kind: 'remote',
          url: 'https://cdn.example/out.png',
          checksum_sha256: 'a'.repeat(64),
          result_index: 1,
        },
      ]),
    ).toEqual([{ url: 'https://cdn.example/out.png', resultIndex: 1 }])
  })

  it('drops storage-internal remote URLs', () => {
    expect(
      extractWorkspaceTaskImageResults(WORKSPACE_ID, [
        { kind: 'remote', url: 'https://cdn.example/ws-uploads/x.png', result_index: 0 },
      ]),
    ).toEqual([])
  })
})

describe('waitCanvasWorkspaceTaskImageResults', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.selectedId.value = WORKSPACE_ID
    resetCanvasWorkspaceTaskRegistryForTests()
  })

  it('settles completed and cancelled-with-results fast paths', async () => {
    mocks.getWorkspaceTask.mockResolvedValueOnce(
      taskItem({
        status: 'completed',
        output_refs: [{ kind: 'asset', asset_id: 'asset_aaa1', role: 'image', result_index: 0 }],
      }),
    )
    await expect(waitCanvasWorkspaceTaskImageResults('task_aaa111')).resolves.toEqual([
      {
        url: '/api/v1/workspaces/ws_test123/media-assets/asset_aaa1/derivatives/preview',
        assetId: 'asset_aaa1',
        resultIndex: 0,
      },
    ])

    mocks.getWorkspaceTask.mockResolvedValueOnce(
      taskItem({
        status: 'cancelled',
        cancel_reason: '用户取消',
        output_refs: [{ kind: 'asset', asset_id: 'asset_bbb2', role: 'image', result_index: 0 }],
      }),
    )
    await expect(waitCanvasWorkspaceTaskImageResults('task_aaa111')).resolves.toMatchObject([
      { assetId: 'asset_bbb2' },
    ])
    expect(mocks.waitWorkspaceTaskTerminal).not.toHaveBeenCalled()
  })

  it('throws terminal failures and propagates 404', async () => {
    mocks.getWorkspaceTask.mockResolvedValueOnce(
      taskItem({ status: 'failed', error_msg: '上游超时' }),
    )
    await expect(waitCanvasWorkspaceTaskImageResults('task_aaa111')).rejects.toThrow('上游超时')

    mocks.getWorkspaceTask.mockRejectedValueOnce(
      Object.assign(new Error('gone'), { status: 404, code: 'task_not_found' }),
    )
    await expect(waitCanvasWorkspaceTaskImageResults('task_aaa111')).rejects.toMatchObject({
      code: 'task_not_found',
    })
  })

  it('waits terminal when the fast path is live and maps timeouts', async () => {
    mocks.getWorkspaceTask.mockResolvedValueOnce(taskItem({ status: 'running' }))
    mocks.waitWorkspaceTaskTerminal.mockResolvedValueOnce(
      taskItem({
        status: 'completed',
        output_refs: [{ kind: 'asset', asset_id: 'asset_ccc3', role: 'image', result_index: 0 }],
      }),
    )
    await expect(waitCanvasWorkspaceTaskImageResults('task_aaa111')).resolves.toMatchObject([
      { assetId: 'asset_ccc3' },
    ])

    mocks.getWorkspaceTask.mockResolvedValueOnce(taskItem({ status: 'running' }))
    mocks.waitWorkspaceTaskTerminal.mockRejectedValueOnce(
      new Error('Workspace task wait timed out'),
    )
    await expect(waitCanvasWorkspaceTaskImageResults('task_aaa111')).rejects.toMatchObject({
      name: 'TaskResultPendingError',
      kind: 'timeout',
    })
  })

  it('fails closed without a workspace selection before any network call', async () => {
    mocks.selectedId.value = null

    await expect(waitCanvasWorkspaceTaskImageResults('task_aaa111')).rejects.toMatchObject({
      name: 'CanvasWorkspaceUnavailableError',
      message: CANVAS_WORKSPACE_REQUIRED_MESSAGE,
    })
    expect(mocks.getWorkspaceTask).not.toHaveBeenCalled()
    expect(mocks.waitWorkspaceTaskTerminal).not.toHaveBeenCalled()
  })

  it('maps waiter-terminal cancelled without results to cancellation', async () => {
    mocks.getWorkspaceTask.mockResolvedValueOnce(taskItem({ status: 'running' }))
    mocks.waitWorkspaceTaskTerminal.mockResolvedValueOnce(
      taskItem({ status: 'cancelled', cancel_reason: '用户取消', output_refs: [] }),
    )
    await expect(waitCanvasWorkspaceTaskImageResults('task_aaa111')).rejects.toMatchObject({
      name: 'TaskCancelledError',
    })
  })

  it('maps waiter aborts to the timeout pending kind', async () => {
    mocks.getWorkspaceTask.mockResolvedValueOnce(taskItem({ status: 'running' }))
    mocks.waitWorkspaceTaskTerminal.mockRejectedValueOnce(new Error('Workspace task wait aborted'))
    await expect(waitCanvasWorkspaceTaskImageResults('task_aaa111')).rejects.toMatchObject({
      name: 'TaskResultPendingError',
      kind: 'timeout',
    })
  })

  it('surfaces waiter-terminal failures unwrapped instead of as timeouts', async () => {
    mocks.getWorkspaceTask.mockResolvedValueOnce(taskItem({ status: 'running' }))
    mocks.waitWorkspaceTaskTerminal.mockResolvedValueOnce(
      taskItem({ status: 'failed', error_msg: '上游失败' }),
    )
    await expect(waitCanvasWorkspaceTaskImageResults('task_aaa111')).rejects.toThrow('上游失败')
  })
})

describe('cancelCanvasWorkspaceTask', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.selectedId.value = WORKSPACE_ID
    resetCanvasWorkspaceTaskRegistryForTests()
  })

  it('cancels in the submit workspace with a deterministic idempotency key', async () => {
    mocks.cancelWorkspaceTask.mockResolvedValueOnce(taskItem({ status: 'cancelled' }))
    registerCanvasWorkspaceTask('task_aaa111', 'ws_other999')
    mocks.selectedId.value = WORKSPACE_ID

    await cancelCanvasWorkspaceTask('task_aaa111')

    expect(mocks.cancelWorkspaceTask).toHaveBeenCalledWith(
      'ws_other999',
      'task_aaa111',
      undefined,
      { idempotencyKey: 'task-cancel:task_aaa111' },
    )
  })

  it('falls back to the current selection and swallows gone/conflict', async () => {
    mocks.cancelWorkspaceTask.mockResolvedValueOnce(taskItem({ status: 'cancelled' }))
    await cancelCanvasWorkspaceTask('task_aaa111', 'user cancel')
    expect(mocks.cancelWorkspaceTask).toHaveBeenCalledWith(
      WORKSPACE_ID,
      'task_aaa111',
      { reason: 'user cancel' },
      { idempotencyKey: 'task-cancel:task_aaa111' },
    )

    mocks.cancelWorkspaceTask.mockRejectedValueOnce(
      Object.assign(new Error('gone'), { status: 404, code: 'task_not_found' }),
    )
    await expect(cancelCanvasWorkspaceTask('task_aaa111')).resolves.toBeUndefined()

    mocks.cancelWorkspaceTask.mockRejectedValueOnce(
      Object.assign(new Error('terminal'), { status: 409, code: 'task_state_conflict' }),
    )
    await expect(cancelCanvasWorkspaceTask('task_aaa111')).resolves.toBeUndefined()
  })

  it('fails closed without any workspace', async () => {
    mocks.selectedId.value = null
    await expect(cancelCanvasWorkspaceTask('task_aaa111')).rejects.toBeInstanceOf(
      CanvasWorkspaceUnavailableError,
    )
    expect(mocks.cancelWorkspaceTask).not.toHaveBeenCalled()
  })
})
