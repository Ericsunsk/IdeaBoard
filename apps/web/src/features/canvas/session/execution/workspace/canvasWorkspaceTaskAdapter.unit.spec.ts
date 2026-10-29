// Canvas workspace task adapter spec (W2 stream 1).
//
// Covers the explicit CanvasTaskRequestData -> v1 semantic mapping: submit input
// mapping + fail-closed rules, quote -> readiness mapping, output-ref image
// extraction, terminal settlement, and cancel workspace resolution. The
// central `api/workspaceTasks` resource is mocked (its own contract
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
  normalizeCanvasOutputResolution,
  submitCanvasWorkspaceTask,
  toSubmitTaskReadiness,
  toWorkspaceCreateInput,
  toWorkspaceQuoteInput,
} from './canvasWorkspaceTaskAdapter'
import { waitCanvasWorkspaceTaskImageOutcome } from './canvasTaskWaiting'

const mocks = vi.hoisted(() => ({
  createWorkspaceTask: vi.fn(),
  quoteWorkspaceTask: vi.fn(),
  getWorkspaceTask: vi.fn(),
  waitWorkspaceTaskTerminal: vi.fn(),
  cancelWorkspaceTask: vi.fn(),
  selectedId: { value: 'ws_test123' as string | null },
  residency: 0,
}))

// Seams import the narrow transport/waiter modules directly (issue #859);
// the mock targets those specifiers so network stays stubbed while the
// shared gone-detection keeps its real behavior.
vi.mock('../../../../../api/workspaceTaskTransport', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../../../../api/workspaceTaskTransport')>()
  return {
    cancelWorkspaceTask: mocks.cancelWorkspaceTask,
    createWorkspaceTask: mocks.createWorkspaceTask,
    getWorkspaceTask: mocks.getWorkspaceTask,
    quoteWorkspaceTask: mocks.quoteWorkspaceTask,
    isWorkspaceTaskGoneError: actual.isWorkspaceTaskGoneError,
  }
})

vi.mock('../../../../../api/workspaceTaskWaiter', () => ({
  waitWorkspaceTaskTerminal: mocks.waitWorkspaceTaskTerminal,
}))

vi.mock('../../../../workspace', () => ({
  getWorkspaceSelectionStore: () => ({
    selectedId: mocks.selectedId,
    getSelectedId: () => mocks.selectedId.value,
  }),
}))

const accessLossMocks = vi.hoisted(() => ({
  isLost: vi.fn(() => false),
  isStale: vi.fn(() => false),
  // Faithful stub of the revalidation-only classifier (the full matrix is
  // pinned in the revocation specs): board capability codes plus bare
  // revalidation 403/404s confirm, everything else preserves.
  isConfirmed: vi.fn((error: unknown) => {
    const record = error as { code?: unknown; status?: unknown } | null
    const code = typeof record?.code === 'string' ? record.code : null
    if (code !== null) {
      return (
        code === 'board_forbidden' || code === 'board_not_found' || code === 'workspace_not_found'
      )
    }
    return record?.status === 403 || record?.status === 404
  }),
  revalidate: vi.fn<() => Promise<'ok' | 'revoked' | 'preserved'>>(async () => 'ok'),
}))

// Closed mock (no importOriginal spread): the real board session boundary
// pulls the workspace selection chain, which this spec stubs closed above.
// The adapter consumes only the public session port.
vi.mock('../../../../board/ports/canvasSession', () => ({
  isBoardAccessLost: accessLossMocks.isLost,
  isBoardAccessStale: accessLossMocks.isStale,
  isConfirmedAccessLoss: accessLossMocks.isConfirmed,
  revalidateAndConvergeBoardAccess: accessLossMocks.revalidate,
  getBoardScope: () => ({
    userScope: 'test-user',
    workspaceId: mocks.selectedId.value,
    residency: mocks.residency,
  }),
  isBoardScopeCurrent: (scope: { residency: number }) => scope.residency === mocks.residency,
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
      output_resolution: '1k',
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

  it('maps board_id bindings and fails closed on forged shapes (#937)', () => {
    expect(
      toWorkspaceCreateInput({ prompt: 'x', service: 'generation', board_id: 'board_abc123' })
        .board_id,
    ).toBe('board_abc123')
    expect(toWorkspaceCreateInput({ prompt: 'x', service: 'generation' }).board_id).toBeUndefined()
    expect(() =>
      toWorkspaceCreateInput({ prompt: 'x', service: 'generation', board_id: 'ws_evil' }),
    ).toThrowError(/board_/)
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
        params: {
          mask_asset_id: 'asset_mask1',
          use_mask: true,
        } satisfies import('../../../core/execution/canvasTaskRequest').CanvasTaskParams,
      }),
    ).toMatchObject({
      service: 'inpaint',
      prompt: 'redraw the area',
      media_asset_ids: ['asset_aaa1'],
      params: {
        mask_asset_id: 'asset_mask1',
        use_mask: true,
      } satisfies import('../../../core/execution/canvasTaskRequest').CanvasTaskParams,
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
          params: {
            mask_image_url: 'data:image/png;base64,AAA',
            use_mask: true,
          } satisfies import('../../../core/execution/canvasTaskRequest').CanvasTaskParams,
        }),
      'media-urls',
    )
    try {
      toWorkspaceCreateInput({
        prompt: '',
        service: 'inpaint',
        params: {
          mask_image_url: 'data:image/png;base64,AAA',
          use_mask: true,
        } satisfies import('../../../core/execution/canvasTaskRequest').CanvasTaskParams,
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
        params: {
          mask_asset_id: ' asset_mask1 ',
          use_mask: true,
        } satisfies import('../../../core/execution/canvasTaskRequest').CanvasTaskParams,
      }).params,
    ).toMatchObject({
      mask_asset_id: 'asset_mask1',
      use_mask: true,
    } satisfies import('../../../core/execution/canvasTaskRequest').CanvasTaskParams)
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

describe('canonical output resolution ingress (#850)', () => {
  it('rejects retired original image params through the unknown-param gate', () => {
    for (const value of ['', 'https://cdn.example/original.png']) {
      expectUnmappable(
        () =>
          toWorkspaceCreateInput({
            prompt: '',
            service: 'render',
            params: { original_image_url: value },
          }),
        'params',
      )
    }
  })

  it('submits service with identical prompt/ratio/dims/count/media semantics', () => {
    const input = toWorkspaceCreateInput({
      prompt: 'a red house',
      service: 'generation',
      media_asset_ids: ['asset_aaa1'],
      params: {
        output_count: 2,
        output_resolution: '2k',
        ratio: '1:1',
        custom_width: 1024,
        custom_height: 1024,
      },
    })
    expect(input.service).toBe('generation')
    expect(input).not.toHaveProperty('workflow_kind')
    expect(input.prompt).toBe('a red house')
    expect(input.media_asset_ids).toEqual(['asset_aaa1'])
    expect(input.params).toMatchObject({
      output_count: 2,
      output_resolution: '2k',
      ratio: '1:1',
      custom_width: 1024,
      custom_height: 1024,
    })
    expect(input.quantity).toBe(2)
    expect(input.resolution).toBe('2k')
  })

  it('normalizes params.output_resolution once and derives the billing resolution from it', () => {
    const input = toWorkspaceCreateInput({
      prompt: 'a cat',
      service: 'generation',
      params: { output_count: 1, output_resolution: '2K' },
    })
    expect(input.params).toMatchObject({ output_resolution: '2k' })
    expect(input.resolution).toBe('2k')
  })

  it('preserves the canonical 1k variant for fixed-resolution services', () => {
    const input = toWorkspaceCreateInput({
      prompt: 'redraw',
      service: 'inpaint',
      params: {
        output_count: 1,
        output_resolution: '1K',
        use_mask: true,
        mask_asset_id: 'asset_mask1',
      } satisfies import('../../../core/execution/canvasTaskRequest').CanvasTaskParams,
    })
    expect(input.params).toMatchObject({ output_resolution: '1k' })
    expect(input.resolution).toBe('1k')
  })

  it('omits resolution when the plan carries none', () => {
    const input = toWorkspaceCreateInput({
      prompt: '',
      service: 'upscale',
      media_asset_ids: ['asset_aaa1'],
      params: { upscale: { scale: 2 } },
    })
    expect(input.params).not.toHaveProperty('output_resolution')
    expect(input).not.toHaveProperty('resolution')
  })

  it('fails closed on unknown resolutions before any network call', () => {
    mocks.createWorkspaceTask.mockClear()
    expectUnmappable(
      () =>
        toWorkspaceCreateInput({
          prompt: 'a cat',
          service: 'generation',
          params: { output_count: 1, output_resolution: '8k' },
        }),
      'params',
    )
    expect(normalizeCanvasOutputResolution(' 4K ')).toBe('4k')
    expect(normalizeCanvasOutputResolution(undefined)).toBeUndefined()
    expect(mocks.createWorkspaceTask).not.toHaveBeenCalled()
  })

  it('canonicalizes quote resolutions from the same source of truth', () => {
    expect(
      toWorkspaceQuoteInput({
        prompt: '',
        service: 'generation',
        params: { output_count: 2, output_resolution: ' 4K ' },
      }),
    ).toEqual({ service: 'generation', quantity: 2, resolution: '4k' })
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

  it('binds board_id onto quote inputs for the shared generate gate (#917)', () => {
    expect(
      toWorkspaceQuoteInput({
        prompt: '',
        service: 'generation',
        board_id: 'board_abc123',
        params: { output_count: 1, output_resolution: '1k' },
      }),
    ).toEqual({ service: 'generation', quantity: 1, resolution: '1k', board_id: 'board_abc123' })
    expect(
      toWorkspaceQuoteInput({
        prompt: '',
        service: 'generation',
        params: { output_count: 1, output_resolution: '1k' },
      }),
    ).toEqual({ service: 'generation', quantity: 1, resolution: '1k' })
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
    } satisfies Partial<
      import('../../../core/execution/canvasTaskRequest').SubmitTaskReadinessData
    >)
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

  it('refuses board-bound submits once access is confirmed lost', async () => {
    accessLossMocks.isLost.mockReturnValueOnce(true)

    await expect(
      submitCanvasWorkspaceTask(
        { prompt: 'a cat', service: 'generation', client_action_id: 'canvas:act-lost' },
        { boardId: 'board_lost1' },
      ),
    ).rejects.toMatchObject({ code: 'board_forbidden' })
    expect(mocks.createWorkspaceTask).not.toHaveBeenCalled()
  })

  it('revalidates stale boards once before submitting', async () => {
    accessLossMocks.isStale.mockReturnValueOnce(true)
    mocks.createWorkspaceTask.mockResolvedValue(taskItem({ status: 'pending' }))

    await expect(
      submitCanvasWorkspaceTask(
        { prompt: 'a cat', service: 'generation', client_action_id: 'canvas:act-stale' },
        { boardId: 'board_stale1' },
      ),
    ).resolves.toEqual({ taskId: 'task_aaa111', workspaceId: WORKSPACE_ID })
    expect(accessLossMocks.revalidate).toHaveBeenCalledWith('board_stale1', {
      workspaceId: WORKSPACE_ID,
    })
    expect(mocks.createWorkspaceTask).toHaveBeenCalledTimes(1)
  })

  it('converges and refuses when stale revalidation proves the loss', async () => {
    accessLossMocks.isStale.mockReturnValueOnce(true)
    accessLossMocks.revalidate.mockResolvedValueOnce('revoked')

    await expect(
      submitCanvasWorkspaceTask(
        { prompt: 'a cat', service: 'generation', client_action_id: 'canvas:act-revoked' },
        { boardId: 'board_revoked1' },
      ),
    ).rejects.toMatchObject({ code: 'board_forbidden' })
    expect(mocks.createWorkspaceTask).not.toHaveBeenCalled()
  })

  it('does not submit or converge a stale revalidation after A→B→A', async () => {
    accessLossMocks.isStale.mockReturnValueOnce(true)
    let resolve!: (verdict: 'revoked') => void
    accessLossMocks.revalidate.mockImplementationOnce(
      () =>
        new Promise((done) => {
          resolve = done
        }),
    )
    const pending = submitCanvasWorkspaceTask(
      { prompt: 'cat', service: 'generation' },
      { boardId: 'board_late' },
    )
    const outcome = expect(pending).rejects.toMatchObject({ code: 'board_scope_changed' })
    await vi.waitFor(() => expect(accessLossMocks.revalidate).toHaveBeenCalled())
    mocks.residency += 2
    resolve('revoked')
    await outcome
    expect(mocks.createWorkspaceTask).not.toHaveBeenCalled()
  })

  it('does not revalidate an old submit failure after A→B→A', async () => {
    let reject!: (error: Error) => void
    mocks.createWorkspaceTask.mockImplementationOnce(
      () =>
        new Promise((_resolve, fail) => {
          reject = fail
        }),
    )
    const pending = submitCanvasWorkspaceTask(
      { prompt: 'cat', service: 'generation' },
      { boardId: 'board_late' },
    )
    const outcome = expect(pending).rejects.toMatchObject({ code: 'board_forbidden' })
    await vi.waitFor(() => expect(mocks.createWorkspaceTask).toHaveBeenCalled())
    mocks.residency += 2
    reject(Object.assign(new Error('denied'), { code: 'board_forbidden' }))
    await outcome
    expect(accessLossMocks.revalidate).not.toHaveBeenCalled()
  })

  it('skips the access gate for board-less submits', async () => {
    accessLossMocks.isLost.mockClear()
    accessLossMocks.isStale.mockClear()
    mocks.createWorkspaceTask.mockResolvedValue(taskItem({ status: 'pending' }))

    await submitCanvasWorkspaceTask({
      prompt: 'a cat',
      service: 'generation',
      client_action_id: 'canvas:act-noboard',
    })

    expect(accessLossMocks.isLost).not.toHaveBeenCalled()
    expect(accessLossMocks.isStale).not.toHaveBeenCalled()
    expect(mocks.createWorkspaceTask).toHaveBeenCalledTimes(1)
  })

  it('revalidates board-bound submit failures and converges on revocation', async () => {
    accessLossMocks.revalidate.mockClear()
    accessLossMocks.revalidate.mockResolvedValueOnce('revoked')
    mocks.createWorkspaceTask.mockRejectedValueOnce(
      Object.assign(new Error('denied'), { status: 403, code: 'board_forbidden' }),
    )

    await expect(
      submitCanvasWorkspaceTask(
        { prompt: 'a cat', service: 'generation', client_action_id: 'canvas:act-submit-403' },
        { boardId: 'board_submit1' },
      ),
    ).rejects.toMatchObject({ code: 'board_forbidden' })
    expect(accessLossMocks.revalidate).toHaveBeenCalledWith('board_submit1', {
      workspaceId: WORKSPACE_ID,
    })
  })

  it('keeps the caller error path when submit revalidation stays unconfirmed', async () => {
    accessLossMocks.revalidate.mockClear()
    mocks.createWorkspaceTask.mockRejectedValueOnce(
      Object.assign(new Error('denied'), { status: 403, code: 'board_forbidden' }),
    )

    await expect(
      submitCanvasWorkspaceTask(
        { prompt: 'a cat', service: 'generation', client_action_id: 'canvas:act-submit-ok' },
        { boardId: 'board_submit2' },
      ),
    ).rejects.toMatchObject({ code: 'board_forbidden' })
    expect(accessLossMocks.revalidate).toHaveBeenCalledWith('board_submit2', {
      workspaceId: WORKSPACE_ID,
    })
  })

  it('skips revalidation for non-capability submit failures', async () => {
    accessLossMocks.revalidate.mockClear()
    mocks.createWorkspaceTask.mockRejectedValueOnce(new Error('socket hang up'))

    await expect(
      submitCanvasWorkspaceTask(
        { prompt: 'a cat', service: 'generation', client_action_id: 'canvas:act-submit-net' },
        { boardId: 'board_submit3' },
      ),
    ).rejects.toThrow('socket hang up')
    expect(accessLossMocks.revalidate).not.toHaveBeenCalled()
  })

  it('maps a 402 quote to insufficient-credits while a 422 quote now throws for submit to surface', async () => {
    // 简化第二波：422/`validation_failed` 不再中转为 not-ready，直接抛出由
    // submit 错误透出（后端全验为准），成功/失败终态不变。
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
    ).rejects.toMatchObject({ status: 422, code: 'validation_failed' })
  })

  it('lets submit-unmappable media ride the quote network with submit surfacing the failure', async () => {
    // 简化第二波：quote 只做 service 白名单最小检查，media-urls/media-ids/
    // storage-URL/data-URL mask 等 submit 口径校验不在 quote 阶段预检；
    // quote 照常走网络（价格探针），submit 侧 toWorkspaceCreateInput 照旧
    // fail-closed，终态同为失败（报错时机从 quote 提前变为 submit 透出）。
    const cases: Array<{ name: string; request: Parameters<typeof toWorkspaceCreateInput>[0] }> = [
      {
        name: 'image_urls',
        request: { prompt: '', service: 'generation', image_urls: ['https://cdn.example/a.png'] },
      },
      {
        name: 'numeric ids',
        request: { prompt: '', service: 'generation', image_asset_ids: [12] },
      },
      {
        name: 'storage URL param',
        request: {
          prompt: '',
          service: 'render',
          params: { fusion: { mode: 'weighted_reference', note: 's3://bucket/x' } },
        },
      },
      {
        name: 'data-URL mask',
        request: {
          prompt: 'redraw',
          service: 'inpaint',
          params: {
            mask_asset_id: 'data:image/png;base64,AAA',
          } satisfies import('../../../core/execution/canvasTaskRequest').CanvasTaskParams,
        },
      },
    ]
    for (const { request } of cases) {
      mocks.quoteWorkspaceTask.mockClear()
      mocks.quoteWorkspaceTask.mockResolvedValueOnce({
        unit_credits: 6,
        total_credits: 6,
        price_version_id: 2,
        price_version: 2,
        quantity: 1,
      })
      await expect(fetchCanvasWorkspaceTaskReadiness(request)).resolves.toMatchObject({
        ready: true,
      } satisfies Partial<
        import('../../../core/execution/canvasTaskRequest').SubmitTaskReadinessData
      >)
      expect(mocks.quoteWorkspaceTask).toHaveBeenCalledTimes(1)
      // submit 侧照旧 fail-closed：同一请求在 submit 映射即抛，后端为准。
      expect(() => toWorkspaceCreateInput(request)).toThrowError(
        expect.objectContaining({ name: 'CanvasWorkspaceTaskUnmappableError' }),
      )
    }
  })

  it('lets full inpaint submits without a mask ride the quote network with submit surfacing the failure', async () => {
    // 简化第二波：inpaint 意图五键推断分支已砍，缺 mask 不再在 quote 阶段以
    // 明确指引标 not-ready；quote 照常走网络，submit/后端 422 透出失败
    //（残留：指引文案不如之前明确）。
    mocks.quoteWorkspaceTask.mockClear()
    mocks.quoteWorkspaceTask.mockResolvedValueOnce({
      unit_credits: 6,
      total_credits: 6,
      price_version_id: 2,
      price_version: 2,
      quantity: 1,
    })
    await expect(
      fetchCanvasWorkspaceTaskReadiness({
        prompt: 'redraw',
        service: 'inpaint',
        media_asset_ids: ['asset_aaa1'],
        params: {
          use_mask: true,
        } satisfies import('../../../core/execution/canvasTaskRequest').CanvasTaskParams,
      }),
    ).resolves.toMatchObject({ ready: true, service: 'inpaint' } satisfies Partial<
      import('../../../core/execution/canvasTaskRequest').SubmitTaskReadinessData
    >)
    expect(mocks.quoteWorkspaceTask).toHaveBeenCalledTimes(1)
  })

  it('keeps generic inpaint price probes quotable without mask context', async () => {
    // 纯价格探针（无 mask 上下文）不受 mask 检查影响，仍走 quote 网络。
    mocks.quoteWorkspaceTask.mockResolvedValueOnce({
      unit_credits: 6,
      total_credits: 6,
      price_version_id: 2,
      price_version: 2,
      quantity: 1,
    })
    await expect(
      fetchCanvasWorkspaceTaskReadiness({
        prompt: '[inpaint] submit readiness',
        service: 'inpaint',
      }),
    ).resolves.toMatchObject({ ready: true, service: 'inpaint' } satisfies Partial<
      import('../../../core/execution/canvasTaskRequest').SubmitTaskReadinessData
    >)
    expect(mocks.quoteWorkspaceTask).toHaveBeenCalledTimes(1)
  })

  it('keeps restricted funds and Board denials not-ready with their codes (#917)', async () => {
    mocks.quoteWorkspaceTask.mockRejectedValueOnce(
      Object.assign(new Error('restricted'), { status: 403, code: 'billing_wallet_restricted' }),
    )
    await expect(
      fetchCanvasWorkspaceTaskReadiness({ prompt: '', service: 'generation' }),
    ).resolves.toMatchObject({
      ready: false,
      service: 'generation',
      error_code: 'wallet_restricted',
    } satisfies Partial<
      import('../../../core/execution/canvasTaskRequest').SubmitTaskReadinessData
    >)

    mocks.quoteWorkspaceTask.mockRejectedValueOnce(
      Object.assign(new Error('viewer'), { status: 403, code: 'task_forbidden' }),
    )
    await expect(
      fetchCanvasWorkspaceTaskReadiness({ prompt: '', service: 'generation' }),
    ).resolves.toMatchObject({
      ready: false,
      service: 'generation',
      error_code: 'task_forbidden',
    } satisfies Partial<
      import('../../../core/execution/canvasTaskRequest').SubmitTaskReadinessData
    >)
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
    } satisfies Partial<
      import('../../../core/execution/canvasTaskRequest').SubmitTaskReadinessData
    >)
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
    expect(items[0]).toMatchObject({ ready: true, service: 'generation' } satisfies Partial<
      import('../../../core/execution/canvasTaskRequest').SubmitTaskReadinessData
    >)
    expect(items[1]).toMatchObject({ ready: true, service: 'cutout' } satisfies Partial<
      import('../../../core/execution/canvasTaskRequest').SubmitTaskReadinessData
    >)
    expect(mocks.quoteWorkspaceTask).toHaveBeenCalledTimes(2)
    await expect(fetchCanvasWorkspaceTaskReadinessMatrix([])).resolves.toEqual([])
  })
})

describe('extractWorkspaceTaskImageResults', () => {
  it('retains Workspace identity for asset refs', () => {
    expect(
      extractWorkspaceTaskImageResults(WORKSPACE_ID, [
        { kind: 'asset', asset_id: 'asset_aaa1', role: 'image', result_index: 0 },
      ]),
    ).toEqual([
      {
        workspaceId: WORKSPACE_ID,
        assetId: 'asset_aaa1',
        resultIndex: 0,
      },
    ])
  })

  it('resolves only strict image roles, dropping blank roles (core/query parity)', () => {
    // 空 role 不再放行 (#P1-7): 无 role 事实的输出引用按“无图像”处理。
    expect(
      extractWorkspaceTaskImageResults(WORKSPACE_ID, [
        {
          kind: 'remote',
          url: 'https://cdn.example/out.png',
          checksum_sha256: 'a'.repeat(64),
          result_index: 1,
        },
      ]),
    ).toEqual([])
    expect(
      extractWorkspaceTaskImageResults(WORKSPACE_ID, [
        {
          kind: 'remote',
          url: 'https://cdn.example/out.png',
          checksum_sha256: 'a'.repeat(64),
          role: 'image',
          result_index: 1,
        },
      ]),
    ).toEqual([{ url: 'https://cdn.example/out.png', resultIndex: 1 }])
  })

  it('drops storage-internal remote URLs', () => {
    expect(
      extractWorkspaceTaskImageResults(WORKSPACE_ID, [
        {
          kind: 'remote',
          url: 'https://cdn.example/ws-uploads/x.png',
          role: 'image',
          result_index: 0,
        },
      ]),
    ).toEqual([])
  })
})

describe('waitCanvasWorkspaceTaskImageOutcome', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.selectedId.value = WORKSPACE_ID
    resetCanvasWorkspaceTaskRegistryForTests()
  })

  it.each(['completed', 'cancelled'] as const)(
    'returns typed %s outcomes with available results',
    async (status) => {
      mocks.waitWorkspaceTaskTerminal.mockResolvedValueOnce(
        taskItem({
          status,
          output_refs: [{ kind: 'asset', asset_id: 'asset_aaa1', role: 'image', result_index: 0 }],
        }),
      )
      await expect(waitCanvasWorkspaceTaskImageOutcome('task_aaa111')).resolves.toEqual({
        kind: status,
        results: [
          {
            workspaceId: WORKSPACE_ID,
            assetId: 'asset_aaa1',
            resultIndex: 0,
          },
        ],
      })
      expect(mocks.getWorkspaceTask).not.toHaveBeenCalled()
    },
  )

  it('returns cancelled without results as a terminal outcome', async () => {
    mocks.waitWorkspaceTaskTerminal.mockResolvedValueOnce(
      taskItem({ status: 'cancelled', output_refs: [] }),
    )
    await expect(waitCanvasWorkspaceTaskImageOutcome('task_aaa111')).resolves.toEqual({
      kind: 'cancelled',
      results: [],
    })
  })

  it('surfaces terminal failure and gone errors unchanged', async () => {
    mocks.waitWorkspaceTaskTerminal.mockResolvedValueOnce(
      taskItem({ status: 'failed', error_msg: '上游超时' }),
    )
    await expect(waitCanvasWorkspaceTaskImageOutcome('task_aaa111')).rejects.toThrow('上游超时')
    mocks.waitWorkspaceTaskTerminal.mockRejectedValueOnce(
      Object.assign(new Error('gone'), { status: 404, code: 'task_not_found' }),
    )
    await expect(waitCanvasWorkspaceTaskImageOutcome('task_aaa111')).rejects.toMatchObject({
      code: 'task_not_found',
    })
  })

  it('maps waiter deadline failures to pending without wrapping terminal failures', async () => {
    mocks.waitWorkspaceTaskTerminal.mockRejectedValueOnce(
      new Error('Workspace task wait timed out'),
    )
    await expect(waitCanvasWorkspaceTaskImageOutcome('task_aaa111')).rejects.toMatchObject({
      name: 'TaskResultPendingError',
      kind: 'timeout',
    })
  })

  it('passes the absolute deadline and signal through to the single waiter', async () => {
    const signal = new AbortController().signal
    const options = { deadlineAt: Date.now() + 500, signal }
    mocks.waitWorkspaceTaskTerminal.mockResolvedValueOnce(taskItem({ status: 'cancelled' }))
    await waitCanvasWorkspaceTaskImageOutcome('task_aaa111', options)
    expect(mocks.waitWorkspaceTaskTerminal).toHaveBeenCalledWith(
      WORKSPACE_ID,
      'task_aaa111',
      options,
    )
  })

  it('fails closed without a workspace selection before any network call', async () => {
    mocks.selectedId.value = null
    await expect(waitCanvasWorkspaceTaskImageOutcome('task_aaa111')).rejects.toMatchObject({
      name: 'CanvasWorkspaceUnavailableError',
      message: CANVAS_WORKSPACE_REQUIRED_MESSAGE,
    })
    expect(mocks.waitWorkspaceTaskTerminal).not.toHaveBeenCalled()
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
