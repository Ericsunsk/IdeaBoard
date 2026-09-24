import { beforeEach, describe, expect, it, vi } from 'vitest'
import { billingRecordsQueryKeys, fetchBillingRecords } from './resource'

const transport = vi.hoisted(() => ({
  request: vi.fn(),
}))

vi.mock('../../../../shared/api/client', () => ({
  api: { request: transport.request },
}))

describe('billingResource', () => {
  beforeEach(() => vi.clearAllMocks())

  it('reads a cursor page through the feature resource boundary', async () => {
    transport.request.mockResolvedValue({
      code: 0,
      data: {
        records: [
          {
            record_id: 'record-1',
            occurred_at: '2026-07-14T08:30:00Z',
            kind: 'ai_usage',
            title: '局部重绘',
            credit_delta: -20,
            available_credits_after: 980,
          },
        ],
        next_before_sequence: 88,
      },
    })

    const controller = new AbortController()
    await expect(
      fetchBillingRecords({ before_sequence: 99, limit: 20 }, controller.signal),
    ).resolves.toMatchObject({
      records: [{ record_id: 'record-1', title: '局部重绘' }],
      next_before_sequence: 88,
    })
    expect(transport.request).toHaveBeenCalledWith(
      '/user/billing/records?before_sequence=99&limit=20',
      expect.objectContaining({ method: 'GET', signal: controller.signal }),
    )
    expect(billingRecordsQueryKeys.all).toEqual(['user', 'billing', 'records'])
  })

  it('surfaces API feedback instead of manufacturing an empty page', async () => {
    transport.request.mockResolvedValue({
      code: 500,
      msg: '账单服务暂不可用',
      message_safe: true,
    })

    await expect(fetchBillingRecords({ limit: 20 })).rejects.toThrow('账单服务暂不可用')
  })

  it('uses the resource fallback when API feedback contains credential details', async () => {
    transport.request.mockResolvedValue({
      code: 500,
      msg: 'credential=exposed',
    })

    await expect(fetchBillingRecords({ limit: 20 })).rejects.toThrow(
      'Could not load billing records.',
    )
  })
})
