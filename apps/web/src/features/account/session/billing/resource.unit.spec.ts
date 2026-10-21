import { beforeEach, describe, expect, it, vi } from 'vitest'
import { billingRecordsQueryKeys, fetchBillingRecords } from './resource'

const transport = vi.hoisted(() => ({
  request: vi.fn(),
}))

vi.mock('../../../../api/client', () => ({
  api: { request: transport.request },
}))

const WORKSPACE_ID = 'ws_123'

describe('billingResource', () => {
  beforeEach(() => vi.clearAllMocks())

  it('reads a cursor page through the v1 public plane', async () => {
    transport.request.mockResolvedValue({
      data: [
        {
          record_id: 'record-1',
          occurred_at: '2026-07-14T08:30:00Z',
          kind: 'ai_usage',
          title: '局部重绘',
          credit_delta: -20,
          available_credits_after: 980,
        },
      ],
      meta: { next_cursor: 'cursor-next', has_more: true },
    })

    const controller = new AbortController()
    await expect(
      fetchBillingRecords(
        WORKSPACE_ID,
        { cursor: 'cursor-prev', page_size: 20 },
        controller.signal,
      ),
    ).resolves.toMatchObject({
      records: [{ record_id: 'record-1', title: '局部重绘' }],
      next_cursor: 'cursor-next',
    })
    expect(transport.request).toHaveBeenCalledWith(
      `/v1/workspaces/${WORKSPACE_ID}/billing-records?cursor=cursor-prev&page_size=20`,
      expect.objectContaining({ method: 'GET', signal: controller.signal }),
    )
    const scope = {
      workspaceId: WORKSPACE_ID,
      billingAccountId: 'ba_1',
      identity: 'user-1',
      consumptionScope: 'account' as const,
    }
    expect(billingRecordsQueryKeys.list(scope)).toEqual(['workspace', 'billing', 'records', scope])
  })

  it('omits the next cursor when the page is exhausted', async () => {
    transport.request.mockResolvedValue({
      data: [],
      meta: { next_cursor: null, has_more: false },
    })

    await expect(fetchBillingRecords(WORKSPACE_ID, { page_size: 20 })).resolves.toMatchObject({
      records: [],
    })
    const page = await fetchBillingRecords(WORKSPACE_ID, { page_size: 20 })
    expect(page.next_cursor).toBeUndefined()
  })

  it('surfaces contract error feedback instead of manufacturing an empty page', async () => {
    transport.request.mockResolvedValue({
      error: { code: 'service_unavailable', message: '账单服务暂不可用' },
    })

    await expect(fetchBillingRecords(WORKSPACE_ID, { page_size: 20 })).rejects.toThrow(
      '账单服务暂不可用',
    )
  })

  it('uses the resource fallback when API feedback contains credential details', async () => {
    transport.request.mockResolvedValue({
      error: { code: 'internal_error', message: 'credential=exposed' },
    })

    await expect(fetchBillingRecords(WORKSPACE_ID, { page_size: 20 })).rejects.toThrow(
      'Could not load billing records.',
    )
  })
})
