import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fetchUsage, usageQueryKeys } from './resource'

const transport = vi.hoisted(() => ({
  request: vi.fn(),
}))

vi.mock('@/api/http/client', () => ({
  api: { request: transport.request },
}))

const WORKSPACE_ID = 'ws_123'

describe('usageResource', () => {
  beforeEach(() => vi.clearAllMocks())

  it('reads lifetime and the server-owned trend through the v1 public plane', async () => {
    transport.request.mockResolvedValue({
      data: {
        timezone: 'Asia/Shanghai',
        lifetime: {
          total_credits: 12,
          peak_daily_credits: 8,
          average_daily_credits: 4,
          task_count: 3,
          active_days: 2,
        },
        trend: [{ date: '2026-07-13', credits: 12 }],
      },
    })

    const controller = new AbortController()
    await expect(fetchUsage(WORKSPACE_ID, '7d', controller.signal)).resolves.toMatchObject({
      lifetime: { total_credits: 12 },
      trend: [{ date: '2026-07-13', credits: 12 }],
    })
    expect(transport.request).toHaveBeenCalledOnce()
    expect(transport.request).toHaveBeenCalledWith(
      `/v1/workspaces/${WORKSPACE_ID}/usage?range=7d`,
      expect.objectContaining({ method: 'GET', signal: controller.signal }),
    )
    const scope = {
      workspaceId: WORKSPACE_ID,
      billingAccountId: 'ba_1',
      identity: 'user-1',
      consumptionScope: 'account' as const,
    }
    expect(
      usageQueryKeys.current(
        scope.workspaceId,
        scope.billingAccountId,
        scope.identity,
        scope.consumptionScope,
        '7d',
      ),
    ).toEqual(['workspace', 'usage', 'current', WORKSPACE_ID, 'ba_1', 'user-1', 'account', '7d'])
  })

  it('surfaces contract error feedback instead of manufacturing an empty trend', async () => {
    transport.request.mockResolvedValue({
      error: { code: 'service_unavailable', message: '用量统计暂不可用' },
    })

    await expect(fetchUsage(WORKSPACE_ID, '30d')).rejects.toThrow('用量统计暂不可用')
  })

  it('uses the resource fallback when API feedback exposes internal transport details', async () => {
    transport.request.mockResolvedValue({
      error: { code: 'internal_error', message: 'dial tcp 127.0.0.1:5432' },
    })

    await expect(fetchUsage(WORKSPACE_ID, '30d')).rejects.toThrow(
      'Could not load usage statistics.',
    )
  })
})
