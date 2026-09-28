import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createAdminRewardCodeResource } from './resource'

const transport = vi.hoisted(() => ({
  request: vi.fn(),
}))

vi.mock('../../../../shared/api/client', () => ({
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

// Reward codes ride the Control Plane with the unified contract envelope
// (`{ data }` / `{ error: { code, message, request_id } }`); mutations
// carry Idempotency-Key + operator reason headers.
describe('AdminRewardCodeResource', () => {
  beforeEach(() => vi.clearAllMocks())

  it('searches codes through the control plane', async () => {
    transport.request.mockResolvedValue(
      contractOk({ reward_codes: [codeFixture()], total: 1, next_before_id: 7 }),
    )
    const resource = createAdminRewardCodeResource()

    const page = await resource.search({ query: '夏日', status: 'enabled', limit: 20 })

    expect(page).toMatchObject({
      rewardCodes: [{ id: 7, displayName: '夏日灵感奖励', status: 'enabled' }],
      total: 1,
      nextBeforeId: 7,
    })
    expect(transport.request).toHaveBeenCalledWith(
      '/control/v1/rewards/codes/search',
      expect.objectContaining({ method: 'POST' }),
    )
    const [, config] = transport.request.mock.calls[0] as [string, { body?: string }]
    expect(JSON.parse(config.body ?? '{}')).toMatchObject({
      query: '夏日',
      status: 'enabled',
      limit: 20,
    })
  })

  it('creates a code with idempotency key and operator reason', async () => {
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
        idempotencyKey: 'reward-create:1',
      },
      { operatorReason: '运营活动补码' },
    )

    expect(created).toMatchObject({
      rewardCode: { id: 7, displayName: '夏日灵感奖励' },
      plaintext: 'IDEA2026',
      replayed: false,
    })
    expect(transport.request).toHaveBeenCalledWith(
      '/control/v1/rewards/codes',
      expect.objectContaining({
        method: 'POST',
        headers: expect.objectContaining({
          'Idempotency-Key': 'reward-create:1',
          'X-Operator-Reason': '运营活动补码',
        }),
      }),
    )
    const [, config] = transport.request.mock.calls[0] as [string, { body?: string }]
    expect(JSON.parse(config.body ?? '{}')).toMatchObject({
      display_name: '夏日灵感奖励',
      code: 'IDEA2026',
      reason: '运营活动补码',
    })
  })

  it('updates a code and rotates plaintext only when a code is supplied', async () => {
    transport.request.mockResolvedValue(
      contractOk({ reward_code: codeFixture(), plaintext: 'NEWCODE1', replayed: false }),
    )
    const resource = createAdminRewardCodeResource()

    const updated = await resource.update(
      7,
      {
        displayName: '新名',
        creditAmount: 80,
        maxRedemptions: 100,
        expiresAt: '',
        code: 'NEWCODE1',
      },
      { operatorReason: '补发 code' },
    )

    expect(updated).toMatchObject({ rewardCode: { id: 7 }, plaintext: 'NEWCODE1' })
    expect(transport.request).toHaveBeenCalledWith(
      '/control/v1/rewards/codes/7',
      expect.objectContaining({ method: 'PATCH' }),
    )
  })

  it('toggles enabled through the control plane with an operator reason', async () => {
    transport.request.mockResolvedValue(
      contractOk(codeFixture({ enabled: false, status: 'disabled' })),
    )
    const resource = createAdminRewardCodeResource()

    const code = await resource.setEnabled(7, false, { operatorReason: '活动结束停用' })

    expect(code).toMatchObject({ id: 7, enabled: false, status: 'disabled' })
    expect(transport.request).toHaveBeenCalledWith(
      '/control/v1/rewards/codes/7/enabled',
      expect.objectContaining({
        method: 'PATCH',
        headers: expect.objectContaining({ 'X-Operator-Reason': '活动结束停用' }),
      }),
    )
  })

  it('deletes a code through the control plane with an operator reason', async () => {
    transport.request.mockResolvedValue(contractOk({ reward_code_id: 7 }))
    const resource = createAdminRewardCodeResource()

    await expect(resource.delete(7, { operatorReason: '误删创建' })).resolves.toBeUndefined()
    expect(transport.request).toHaveBeenCalledWith(
      '/control/v1/rewards/codes/7',
      expect.objectContaining({
        method: 'DELETE',
        headers: expect.objectContaining({ 'X-Operator-Reason': '误删创建' }),
      }),
    )
  })

  it('requires an operator reason for mutations before any network call', async () => {
    const resource = createAdminRewardCodeResource()

    await expect(resource.setEnabled(7, false, { operatorReason: '  ' })).rejects.toMatchObject({
      code: 'control_reason_required',
    })
    await expect(resource.delete(7, { operatorReason: '' })).rejects.toMatchObject({
      code: 'control_reason_required',
    })
    expect(transport.request).not.toHaveBeenCalled()
  })
})
