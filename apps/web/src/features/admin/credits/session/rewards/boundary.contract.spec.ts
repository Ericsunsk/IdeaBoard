import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createAdminRewardCodeResource } from './resource'

const transport = vi.hoisted(() => ({
  request: vi.fn(),
}))

vi.mock('../../../../../api/client', () => ({
  api: { request: transport.request },
}))

function codeFixture(overrides: Record<string, unknown> = {}): Record<string, unknown> {
  return {
    id: 7,
    display_name: '夏日灵感奖励',
    masked_code_hint: '•••• 2922',
    credit_amount: 80,
    max_redemptions: 100,
    redemption_count: 3,
    expires_at: '2026-12-31T00:00:00Z',
    enabled: true,
    status: 'enabled',
    created_at: '2026-07-01T00:00:00Z',
    updated_at: '2026-07-15T00:00:00Z',
    ...overrides,
  }
}

function contractOk(data: unknown): Record<string, unknown> {
  return { data }
}

function forbiddenError(): Error & { code?: unknown; status?: unknown } {
  return Object.assign(new Error('denied'), { code: 'forbidden', status: 403 })
}

// Boundary review (Generation CONTEXT vs credits/server boundary):
// - Generation `core/contracts/generationContract.ts` is strict fail-closed:
//   `parse*` returns null on invalid, `assert*` throws, never clamps/coerces;
//   snapshot repair (`normalizeOutputResolution`) is a separate seam, never a
//   catalog-write validation.
// - Credits `rewards/resource.ts` was lenient: blank idempotencyKey silently
//   minted a fallback (`trim() || controlCreditIdempotencyKey`), past
//   `expiresAt` passed straight to the wire, and 403/`forbidden` fell through
//   `withControlCopy` to the raw transport message.
// - This spec pins the strict fail-before: blank keys and expired inputs
//   reject with machine codes before any network call; 403 maps to approved
//   operator copy with no plaintext leak; valid intents keep header-only
//   identities unchanged through the vertical seam.
describe('credits/server boundary idempotency/expiry/403', () => {
  beforeEach(() => vi.clearAllMocks())

  it('rejects blank idempotency keys before any network call', async () => {
    transport.request.mockResolvedValue(
      contractOk({ reward_code: codeFixture(), plaintext: 'IDEA2026', replayed: false }),
    )
    const resource = createAdminRewardCodeResource()

    await expect(
      resource.create(
        {
          displayName: '夏日灵感奖励',
          code: 'IDEA2026',
          creditAmount: 80,
          maxRedemptions: 100,
          expiresAt: '2026-12-31T00:00:00Z',
          idempotencyKey: '   ',
        },
        { operatorReason: '运营活动补码' },
      ),
    ).rejects.toMatchObject({ code: 'control_idempotency_required' })
    expect(transport.request).not.toHaveBeenCalled()
  })

  it('rejects expired inputs before any network call', async () => {
    transport.request.mockResolvedValue(
      contractOk({ reward_code: codeFixture(), plaintext: 'IDEA2026', replayed: false }),
    )
    const resource = createAdminRewardCodeResource()

    await expect(
      resource.create(
        {
          displayName: '夏日灵感奖励',
          code: 'IDEA2026',
          creditAmount: 80,
          maxRedemptions: 100,
          expiresAt: '2000-01-01T00:00:00Z',
          idempotencyKey: 'reward-create:expired-1',
        },
        { operatorReason: '运营活动补码' },
      ),
    ).rejects.toMatchObject({ code: 'control_expired_input' })
    expect(transport.request).not.toHaveBeenCalled()
  })

  it('maps 403 to operator copy without leaking plaintext', async () => {
    transport.request.mockRejectedValue(forbiddenError())
    const resource = createAdminRewardCodeResource()

    const failure = await resource
      .create(
        {
          displayName: '夏日灵感奖励',
          code: 'IDEA2026',
          creditAmount: 80,
          maxRedemptions: 100,
          expiresAt: '2026-12-31T00:00:00Z',
          idempotencyKey: 'reward-create:403-1',
        },
        { operatorReason: '运营活动补码' },
      )
      .then(
        () => {
          throw new Error('expected create to reject on 403')
        },
        (error: unknown) => error,
      )

    expect(failure).toMatchObject({ code: 'forbidden' })
    expect((failure as Error).message).toBe('权限不足，无法操作兑换码')
    // No success side-effect: no plaintext, no replay marker on failure.
    expect(failure).not.toHaveProperty('plaintext')
    expect(transport.request).toHaveBeenCalledTimes(1)
  })

  it('keeps header-only identities unchanged for valid intents', async () => {
    transport.request.mockResolvedValue(
      contractOk({ reward_code: codeFixture(), plaintext: 'IDEA2026', replayed: false }),
    )
    const resource = createAdminRewardCodeResource()

    const created = await resource.create(
      {
        displayName: '夏日灵感奖励',
        code: 'IDEA2026',
        creditAmount: 80,
        maxRedemptions: 100,
        expiresAt: '2026-12-31T00:00:00Z',
        idempotencyKey: 'reward-create:seam-1',
      },
      { operatorReason: '运营活动补码' },
    )

    expect(created.plaintext).toBe('IDEA2026')
    expect(transport.request).toHaveBeenCalledTimes(1)
    const [, config] = transport.request.mock.calls[0] as [
      string,
      { body?: string; headers?: Record<string, string> },
    ]
    expect(config.headers).toMatchObject({
      'Idempotency-Key': 'reward-create:seam-1',
      'X-Operator-Reason': '运营活动补码',
    })
    expect(JSON.parse(config.body ?? '{}')).not.toHaveProperty('idempotency_key')
  })
})
