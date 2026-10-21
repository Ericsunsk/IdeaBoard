import { beforeEach, describe, expect, test, vi } from 'vitest'
import {
  DEFAULT_QUOTA_PRESETS,
  defaultMemberQuotaQueryKeys,
  fetchDefaultMemberQuota,
  fetchMemberQuotaList,
  fetchSelfMemberQuota,
  isQuotaExhausted,
  MAX_MEMBER_QUOTA_LIMIT,
  memberQuotaQueryKeys,
  normalizeQuotaInput,
  parseDefaultMemberQuotaData,
  parseMemberQuotaData,
  parseMemberQuotaList,
  updateDefaultMemberQuota,
  updateMemberQuota,
} from './resource'
import { WORKSPACE_SELECTION_REQUIRED_CODE } from '../workspaceBilling'

const transport = vi.hoisted(() => ({
  request: vi.fn(),
}))

vi.mock('../../../../api/client', () => ({
  api: { request: transport.request },
}))

const WORKSPACE_ID = 'ws_123'
const MEMBER_ID = 'usr_abc123'

function dailyBody(overrides: Record<string, unknown> = {}) {
  return {
    data: {
      billing_account_id: 'ba_1',
      user_id: MEMBER_ID,
      period: 'daily',
      limit_credits: 100,
      used_credits: 25,
      remaining_credits: 75,
      period_start: '2026-10-21T00:00:00Z',
      period_end: '2026-10-22T00:00:00Z',
      ...overrides,
    },
  }
}

describe('memberQuotaResource', () => {
  beforeEach(() => vi.clearAllMocks())

  test('parses a daily quota with usage and remaining', () => {
    expect(parseMemberQuotaData(dailyBody().data)).toEqual({
      billingAccountId: 'ba_1',
      userId: MEMBER_ID,
      period: 'daily',
      limitCredits: 100,
      usedCredits: 25,
      remainingCredits: 75,
      periodStart: '2026-10-21T00:00:00Z',
      periodEnd: '2026-10-22T00:00:00Z',
    })
  })

  test('parses an unlimited quota with no cap fields', () => {
    expect(
      parseMemberQuotaData({
        billing_account_id: 'ba_1',
        user_id: MEMBER_ID,
        period: 'unlimited',
        used_credits: 40,
      }),
    ).toEqual({
      billingAccountId: 'ba_1',
      userId: MEMBER_ID,
      period: 'unlimited',
      limitCredits: null,
      usedCredits: 40,
      remainingCredits: null,
      periodStart: null,
      periodEnd: null,
    })
  })

  test('rejects mismatched period/limit/user shapes', () => {
    expect(() => parseMemberQuotaData(dailyBody({ limit_credits: undefined }).data)).toThrow()
    expect(() => parseMemberQuotaData(dailyBody({ period: 'unlimited' }).data)).toThrow()
    expect(() => parseMemberQuotaData(dailyBody({ period: 'yearly' }).data)).toThrow()
    expect(() => parseMemberQuotaData(dailyBody({ user_id: 'ws_1' }).data)).toThrow()
    expect(() => parseMemberQuotaData(dailyBody({ billing_account_id: 'ws_1' }).data)).toThrow()
  })

  test('detects exhaustion only on capped quotas at the limit', () => {
    expect(isQuotaExhausted(parseMemberQuotaData(dailyBody({ used_credits: 100 }).data))).toBe(true)
    expect(isQuotaExhausted(parseMemberQuotaData(dailyBody().data))).toBe(false)
    expect(
      isQuotaExhausted(
        parseMemberQuotaData({
          billing_account_id: 'ba_1',
          user_id: MEMBER_ID,
          period: 'unlimited',
          used_credits: 999999,
        }),
      ),
    ).toBe(false)
  })

  test('parses a quota list body', () => {
    expect(parseMemberQuotaList([dailyBody().data])).toEqual([
      parseMemberQuotaData(dailyBody().data),
    ])
    expect(() => parseMemberQuotaList(dailyBody().data)).toThrow()
  })

  test('normalizes set input to the wire shape', () => {
    expect(normalizeQuotaInput({ period: 'weekly', limitCredits: 500 })).toEqual({
      period: 'weekly',
      limit_credits: 500,
    })
    expect(normalizeQuotaInput({ period: 'fixed', limitCredits: 500 })).toEqual({
      period: 'fixed',
      limit_credits: 500,
    })
    expect(normalizeQuotaInput({ period: 'unlimited' })).toEqual({ period: 'unlimited' })
    expect(() => normalizeQuotaInput({ period: 'daily', limitCredits: 0 })).toThrow()
    expect(() =>
      normalizeQuotaInput({ period: 'monthly', limitCredits: MAX_MEMBER_QUOTA_LIMIT + 1 }),
    ).toThrow()
  })

  test('fetches the self quota through the workspace billing plane', async () => {
    transport.request.mockResolvedValue(dailyBody())
    const controller = new AbortController()
    await expect(fetchSelfMemberQuota(WORKSPACE_ID, controller.signal)).resolves.toMatchObject({
      userId: MEMBER_ID,
      period: 'daily',
      limitCredits: 100,
    })
    expect(transport.request).toHaveBeenCalledWith(
      `/v1/workspaces/${WORKSPACE_ID}/member-quotas/self`,
      expect.objectContaining({ method: 'GET', signal: controller.signal }),
    )
    expect(memberQuotaQueryKeys.self(WORKSPACE_ID)).toEqual([
      'workspace',
      'member-quota',
      'self',
      WORKSPACE_ID,
    ])
  })

  test('fetches the manager list through the workspace billing plane', async () => {
    transport.request.mockResolvedValue({ data: [dailyBody().data] })
    await expect(fetchMemberQuotaList(WORKSPACE_ID)).resolves.toHaveLength(1)
    expect(transport.request).toHaveBeenCalledWith(
      `/v1/workspaces/${WORKSPACE_ID}/member-quotas`,
      expect.objectContaining({ method: 'GET' }),
    )
    expect(memberQuotaQueryKeys.list(WORKSPACE_ID)).toEqual([
      'workspace',
      'member-quota',
      'list',
      WORKSPACE_ID,
    ])
  })

  test('updates through the workspace billing plane without idempotency keys', async () => {
    transport.request.mockResolvedValue(dailyBody({ limit_credits: 50, remaining_credits: 25 }))
    await expect(
      updateMemberQuota(WORKSPACE_ID, MEMBER_ID, { period: 'daily', limitCredits: 50 }),
    ).resolves.toMatchObject({ limitCredits: 50 })
    expect(transport.request).toHaveBeenCalledWith(
      `/v1/workspaces/${WORKSPACE_ID}/member-quotas/${MEMBER_ID}`,
      expect.objectContaining({ method: 'PUT' }),
    )
    const [, options] = transport.request.mock.calls[0] as [string, Record<string, unknown>]
    expect(options.headers).not.toMatchObject({ 'Idempotency-Key': expect.anything() })
  })

  test('requires a workspace selection', async () => {
    await expect(fetchSelfMemberQuota('')).rejects.toMatchObject({
      code: WORKSPACE_SELECTION_REQUIRED_CODE,
    })
    await expect(fetchMemberQuotaList('')).rejects.toMatchObject({
      code: WORKSPACE_SELECTION_REQUIRED_CODE,
    })
  })

  test('parses a monthly default with the account scope', () => {
    expect(
      parseDefaultMemberQuotaData({
        billing_account_id: 'ba_1',
        workspace_id: WORKSPACE_ID,
        period: 'monthly',
        limit_credits: 1000,
      }),
    ).toEqual({
      billingAccountId: 'ba_1',
      workspaceId: WORKSPACE_ID,
      period: 'monthly',
      limitCredits: 1000,
    })
  })

  test('parses an unlimited default with no limit', () => {
    expect(
      parseDefaultMemberQuotaData({
        billing_account_id: 'ba_1',
        workspace_id: WORKSPACE_ID,
        period: 'unlimited',
      }),
    ).toEqual({
      billingAccountId: 'ba_1',
      workspaceId: WORKSPACE_ID,
      period: 'unlimited',
      limitCredits: null,
    })
  })

  test('rejects mismatched default shapes', () => {
    expect(() =>
      parseDefaultMemberQuotaData({
        billing_account_id: 'ba_1',
        workspace_id: WORKSPACE_ID,
        period: 'monthly',
      }),
    ).toThrow()
    expect(() =>
      parseDefaultMemberQuotaData({
        billing_account_id: 'ba_1',
        workspace_id: WORKSPACE_ID,
        period: 'unlimited',
        limit_credits: 100,
      }),
    ).toThrow()
    expect(() =>
      parseDefaultMemberQuotaData({
        billing_account_id: 'ws_1',
        workspace_id: WORKSPACE_ID,
        period: 'monthly',
        limit_credits: 100,
      }),
    ).toThrow()
  })

  test('exposes the small/medium/large monthly presets', () => {
    expect([...DEFAULT_QUOTA_PRESETS]).toEqual([200, 1000, 2000])
  })

  test('fetches the default through the workspace billing plane', async () => {
    transport.request.mockResolvedValue({
      data: {
        billing_account_id: 'ba_1',
        workspace_id: WORKSPACE_ID,
        period: 'monthly',
        limit_credits: 1000,
      },
    })
    await expect(fetchDefaultMemberQuota(WORKSPACE_ID)).resolves.toMatchObject({
      period: 'monthly',
      limitCredits: 1000,
    })
    expect(transport.request).toHaveBeenCalledWith(
      `/v1/workspaces/${WORKSPACE_ID}/default-member-quota`,
      expect.objectContaining({ method: 'GET' }),
    )
    expect(defaultMemberQuotaQueryKeys.detail(WORKSPACE_ID)).toEqual([
      'workspace',
      'default-member-quota',
      'detail',
      WORKSPACE_ID,
    ])
  })

  test('updates the default without idempotency keys', async () => {
    transport.request.mockResolvedValue({
      data: {
        billing_account_id: 'ba_1',
        workspace_id: WORKSPACE_ID,
        period: 'monthly',
        limit_credits: 200,
      },
    })
    await expect(
      updateDefaultMemberQuota(WORKSPACE_ID, { period: 'monthly', limitCredits: 200 }),
    ).resolves.toMatchObject({ limitCredits: 200 })
    expect(transport.request).toHaveBeenCalledWith(
      `/v1/workspaces/${WORKSPACE_ID}/default-member-quota`,
      expect.objectContaining({ method: 'PUT' }),
    )
    const [, options] = transport.request.mock.calls[0] as [string, Record<string, unknown>]
    expect(options.headers).not.toMatchObject({ 'Idempotency-Key': expect.anything() })
  })
})
