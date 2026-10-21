import { beforeEach, describe, expect, test, vi } from 'vitest'
import {
  fetchWorkspaceBudget,
  isBudgetExhausted,
  normalizeBudgetInput,
  parseWorkspaceBudgetData,
  updateWorkspaceBudget,
  workspaceBudgetQueryKeys,
  MAX_WORKSPACE_BUDGET_LIMIT,
} from './resource'
import { WORKSPACE_SELECTION_REQUIRED_CODE } from '../workspaceBilling'

const transport = vi.hoisted(() => ({
  request: vi.fn(),
}))

vi.mock('../../../../api/client', () => ({
  api: { request: transport.request },
}))

const WORKSPACE_ID = 'ws_123'

function monthlyBody(overrides: Record<string, unknown> = {}) {
  return {
    data: {
      workspace_id: WORKSPACE_ID,
      billing_account_id: 'ba_1',
      period: 'monthly',
      limit_credits: 1000,
      used_credits: 250,
      remaining_credits: 750,
      period_start: '2026-10-01T00:00:00Z',
      period_end: '2026-11-01T00:00:00Z',
      ...overrides,
    },
  }
}

describe('workspaceBudgetResource', () => {
  beforeEach(() => vi.clearAllMocks())

  test('parses a monthly budget with usage and remaining', () => {
    expect(parseWorkspaceBudgetData(monthlyBody().data)).toEqual({
      workspaceId: WORKSPACE_ID,
      billingAccountId: 'ba_1',
      period: 'monthly',
      limitCredits: 1000,
      usedCredits: 250,
      remainingCredits: 750,
      periodStart: '2026-10-01T00:00:00Z',
      periodEnd: '2026-11-01T00:00:00Z',
    })
  })

  test('parses an unlimited budget with no cap fields', () => {
    expect(
      parseWorkspaceBudgetData({
        workspace_id: WORKSPACE_ID,
        billing_account_id: 'ba_1',
        period: 'unlimited',
        used_credits: 40,
      }),
    ).toEqual({
      workspaceId: WORKSPACE_ID,
      billingAccountId: 'ba_1',
      period: 'unlimited',
      limitCredits: null,
      usedCredits: 40,
      remainingCredits: null,
      periodStart: null,
      periodEnd: null,
    })
  })

  test('rejects mismatched period/limit shapes', () => {
    expect(() => parseWorkspaceBudgetData(monthlyBody({ limit_credits: undefined }).data)).toThrow()
    expect(() => parseWorkspaceBudgetData(monthlyBody({ period: 'unlimited' }).data)).toThrow()
    expect(() => parseWorkspaceBudgetData(monthlyBody({ period: 'weekly' }).data)).toThrow()
    expect(() =>
      parseWorkspaceBudgetData(monthlyBody({ billing_account_id: 'ws_1' }).data),
    ).toThrow()
  })

  test('detects exhaustion only on capped budgets at the limit', () => {
    expect(
      isBudgetExhausted(parseWorkspaceBudgetData(monthlyBody({ used_credits: 1000 }).data)),
    ).toBe(true)
    expect(isBudgetExhausted(parseWorkspaceBudgetData(monthlyBody().data))).toBe(false)
    expect(
      isBudgetExhausted(
        parseWorkspaceBudgetData({
          workspace_id: WORKSPACE_ID,
          billing_account_id: 'ba_1',
          period: 'unlimited',
          used_credits: 999999,
        }),
      ),
    ).toBe(false)
  })

  test('normalizes set input to the wire shape', () => {
    expect(normalizeBudgetInput({ period: 'monthly', limitCredits: 500 })).toEqual({
      period: 'monthly',
      limit_credits: 500,
    })
    expect(normalizeBudgetInput({ period: 'unlimited' })).toEqual({ period: 'unlimited' })
    expect(() => normalizeBudgetInput({ period: 'monthly', limitCredits: 0 })).toThrow()
    expect(() =>
      normalizeBudgetInput({ period: 'monthly', limitCredits: MAX_WORKSPACE_BUDGET_LIMIT + 1 }),
    ).toThrow()
  })

  test('fetches through the workspace billing plane', async () => {
    transport.request.mockResolvedValue(monthlyBody())
    const controller = new AbortController()
    await expect(fetchWorkspaceBudget(WORKSPACE_ID, controller.signal)).resolves.toMatchObject({
      workspaceId: WORKSPACE_ID,
      period: 'monthly',
      limitCredits: 1000,
    })
    expect(transport.request).toHaveBeenCalledWith(
      `/v1/workspaces/${WORKSPACE_ID}/budget`,
      expect.objectContaining({ method: 'GET', signal: controller.signal }),
    )
    expect(workspaceBudgetQueryKeys.detail(WORKSPACE_ID)).toEqual([
      'workspace',
      'budget',
      'detail',
      WORKSPACE_ID,
    ])
  })

  test('updates through the workspace billing plane without idempotency keys', async () => {
    transport.request.mockResolvedValue(monthlyBody({ limit_credits: 500, remaining_credits: 250 }))
    await expect(
      updateWorkspaceBudget(WORKSPACE_ID, { period: 'monthly', limitCredits: 500 }),
    ).resolves.toMatchObject({ limitCredits: 500 })
    expect(transport.request).toHaveBeenCalledWith(
      `/v1/workspaces/${WORKSPACE_ID}/budget`,
      expect.objectContaining({ method: 'PUT' }),
    )
    const [, options] = transport.request.mock.calls[0] as [string, Record<string, unknown>]
    expect(options.headers).not.toMatchObject({ 'Idempotency-Key': expect.anything() })
  })

  test('requires a workspace selection', async () => {
    await expect(fetchWorkspaceBudget('')).rejects.toMatchObject({
      code: WORKSPACE_SELECTION_REQUIRED_CODE,
    })
  })
})
