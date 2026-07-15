import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createAdminRewardCodeBatchResource } from './rewardCodeBatchResource'

const generated = vi.hoisted(() => ({
  generate: vi.fn(),
  list: vi.fn(),
  revoke: vi.fn(),
}))

vi.mock('../../../../shared/api/generated-query/admin-rewards/admin-rewards', () => ({
  generateAdminRewardCodeBatch: generated.generate,
  listAdminRewardCodeBatches: generated.list,
  revokeAdminRewardCodeBatch: generated.revoke,
}))

const batch = {
  id: 21,
  campaign_id: 7,
  idempotency_key: 'reward-batch:stable-key',
  code_count: 3,
  valid_unused_count: 2,
  redeemed_count: 1,
  revoked_unused_count: 0,
  issued_by: 1,
  issued_at: '2026-07-16T01:00:00Z',
}

describe('AdminRewardCodeBatchResource', () => {
  beforeEach(() => vi.clearAllMocks())

  it('maps keyset Batch metadata and counts without plaintext', async () => {
    generated.list.mockResolvedValue({
      code: 0,
      msg: 'ok',
      data: { batches: [batch], next_before_id: 20 },
    })

    const page = await createAdminRewardCodeBatchResource().list(7, 30)

    expect(generated.list).toHaveBeenCalledWith(7, { before_id: 30, limit: 20 })
    expect(page).toEqual({
      batches: [
        {
          id: 21,
          campaignId: 7,
          idempotencyKey: 'reward-batch:stable-key',
          codeCount: 3,
          validUnusedCount: 2,
          redeemedCount: 1,
          revokedUnusedCount: 0,
          issuedBy: 1,
          issuedAt: '2026-07-16T01:00:00Z',
          revokedAt: undefined,
          revokedBy: undefined,
        },
      ],
      nextBeforeId: 20,
    })
  })

  it('preserves the caller idempotency key and returns plaintext only from the first response', async () => {
    generated.generate.mockResolvedValueOnce({
      code: 0,
      msg: 'ok',
      data: {
        batch,
        plaintexts: ['ABCD-EFGH-JKMN-PQRS', 'BCDE-FGHJ-KMNP-QRST', 'CDEF-GHJK-MNPQ-RSTV'],
        export_unavailable: false,
        replayed: false,
      },
    })
    const resource = createAdminRewardCodeBatchResource()

    const result = await resource.generate(7, {
      count: 3,
      idempotencyKey: ' reward-batch:stable-key ',
      reason: ' 首次发行 ',
    })

    expect(generated.generate).toHaveBeenCalledWith(7, {
      count: 3,
      idempotency_key: 'reward-batch:stable-key',
      reason: '首次发行',
    })
    expect(result).toEqual(
      expect.objectContaining({
        batch: expect.objectContaining({ id: 21, codeCount: 3 }),
        plaintexts: ['ABCD-EFGH-JKMN-PQRS', 'BCDE-FGHJ-KMNP-QRST', 'CDEF-GHJK-MNPQ-RSTV'],
        exportUnavailable: false,
        replayed: false,
      }),
    )
  })

  it('never fabricates plaintext for an idempotent replay and revokes with confirmation', async () => {
    generated.generate.mockResolvedValue({
      code: 0,
      msg: 'ok',
      data: { batch, export_unavailable: true, replayed: true },
    })
    generated.revoke.mockResolvedValue({
      code: 0,
      msg: 'ok',
      data: {
        ...batch,
        valid_unused_count: 0,
        revoked_unused_count: 2,
        revoked_at: '2026-07-16T02:00:00Z',
        revoked_by: 1,
      },
    })
    const resource = createAdminRewardCodeBatchResource()

    const replay = await resource.generate(7, {
      count: 3,
      idempotencyKey: 'reward-batch:stable-key',
      reason: '重试发行',
    })
    const revoked = await resource.revoke(7, 21, ' 撤销丢失导出的 Batch ')

    expect(replay.plaintexts).toEqual([])
    expect(replay.exportUnavailable).toBe(true)
    expect(replay.replayed).toBe(true)
    expect(generated.revoke).toHaveBeenCalledWith(7, 21, {
      confirmed: true,
      reason: '撤销丢失导出的 Batch',
    })
    expect(revoked).toEqual(expect.objectContaining({ validUnusedCount: 0, revokedUnusedCount: 2 }))
  })
})
