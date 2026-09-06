import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fetchUsage, usageQueryKeys } from './usageResource'

const generated = vi.hoisted(() => ({
  getUserUsage: vi.fn(),
}))

vi.mock('../../../../shared/api/generated-query/user-usage/user-usage', () => generated)

describe('usageResource', () => {
  beforeEach(() => vi.clearAllMocks())

  it('reads lifetime and the server-owned trend in one authenticated request', async () => {
    generated.getUserUsage.mockResolvedValue({
      code: 0,
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
    await expect(fetchUsage('7d', controller.signal)).resolves.toMatchObject({
      lifetime: { total_credits: 12 },
      trend: [{ date: '2026-07-13', credits: 12 }],
    })
    expect(generated.getUserUsage).toHaveBeenCalledOnce()
    expect(generated.getUserUsage).toHaveBeenCalledWith(
      { range: '7d' },
      { signal: controller.signal },
    )
    expect(usageQueryKeys.current('7d')).toEqual(['user', 'usage', 'current', '7d'])
  })

  it('surfaces API feedback instead of manufacturing an empty trend', async () => {
    generated.getUserUsage.mockResolvedValue({
      code: 500,
      msg: '用量统计暂不可用',
      message_safe: true,
    })

    await expect(fetchUsage('30d')).rejects.toThrow('用量统计暂不可用')
  })

  it('uses the resource fallback when API feedback exposes internal transport details', async () => {
    generated.getUserUsage.mockResolvedValue({ code: 500, msg: 'dial tcp 127.0.0.1:5432' })

    await expect(fetchUsage('30d')).rejects.toThrow('Could not load usage statistics.')
  })
})
