import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fetchUserUsage } from './userUsageResource'

const generated = vi.hoisted(() => ({
  getUserUsage: vi.fn(),
}))

vi.mock('../../shared/api/generated-query/user-usage/user-usage', () => generated)

describe('userUsageResource', () => {
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

    await expect(fetchUserUsage()).resolves.toMatchObject({
      lifetime: { total_credits: 12 },
      trend: [{ date: '2026-07-13', credits: 12 }],
    })
    expect(generated.getUserUsage).toHaveBeenCalledOnce()
    expect(generated.getUserUsage).toHaveBeenCalledWith()
  })

  it('surfaces API feedback instead of manufacturing an empty trend', async () => {
    generated.getUserUsage.mockResolvedValue({ code: 500, msg: '用量统计暂不可用' })

    await expect(fetchUserUsage()).rejects.toThrow('用量统计暂不可用')
  })
})
