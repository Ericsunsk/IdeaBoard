import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { renderWithUser, screen, waitFor } from '../../../../test/render'
import RewardCodeRedemptionsDialog from './RewardCodeRedemptionsDialog.vue'
import type { RewardCode } from './resource'

// The redemption transport rides the Control Plane; dialog UX keeps
// coverage through a fake resource while the resource spec pins the
// control contract.
const redemptionFakes = vi.hoisted(() => ({
  list: vi.fn(),
  get: vi.fn(),
}))

vi.mock('./rewardCodeRedemptionResource', async (importOriginal) => {
  const actual = await importOriginal<
    typeof import('./rewardCodeRedemptionResource')
  >()
  return {
    ...actual,
    createRewardCodeRedemptionResource: () => ({
      list: redemptionFakes.list,
      get: redemptionFakes.get,
    }),
  }
})

describe('RewardCodeRedemptionsDialog', () => {
  beforeEach(() => {
    redemptionFakes.list.mockReset()
    redemptionFakes.get.mockReset()
  })

  it('loads a dense Redemption page through the feature resource seam', async () => {
    const calls: Array<{ rewardCodeId: number; beforeId: number | undefined }> = []
    redemptionFakes.list.mockImplementation(async (rewardCodeId: number, input?: object) => {
      const beforeId = (input as { beforeId?: number }).beforeId
      calls.push({ rewardCodeId, beforeId })
      return viewPage([makeViewSummary()])
    })
    redemptionFakes.get.mockResolvedValue(makeViewDetail())

    renderDialog()

    expect(await screen.findByText('用户 #23')).toBeVisible()
    expect(screen.getByText('80 Ideas')).toBeVisible()
    expect(screen.getByText(/2026/)).toBeVisible()
    expect(screen.queryByText('Redemption ID')).not.toBeInTheDocument()
    expect(screen.queryByText('masked Code hint')).not.toBeInTheDocument()
    expect(calls).toEqual([{ rewardCodeId: 7, beforeId: undefined }])
  })

  it('loads long technical detail only after the row is expanded', async () => {
    let releaseDetail!: () => void
    const detailGate = new Promise<void>((resolve) => {
      releaseDetail = resolve
    })
    const detailCalls: Array<{ rewardCodeId: number; redemptionId: number }> = []
    redemptionFakes.list.mockResolvedValue(viewPage([makeViewSummary()]))
    redemptionFakes.get.mockImplementation(async (rewardCodeId: number, redemptionId: number) => {
      detailCalls.push({ rewardCodeId, redemptionId })
      await detailGate
      return makeViewDetail()
    })
    const { user } = renderDialog()
    await screen.findByText('用户 #23')

    expect(detailCalls).toEqual([])
    await user.click(screen.getByRole('button', { name: /详情/ }))
    expect(screen.getByLabelText('加载兑换详情')).toBeVisible()
    releaseDetail()

    expect(await screen.findByText('Redemption ID')).toBeVisible()
    expect(screen.getByText('Credits Transaction ID')).toBeVisible()
    expect(screen.getByText('masked Code hint')).toBeVisible()
    expect(screen.getByText('credit-transaction-9001-with-a-long-audit-reference')).toBeVisible()
    expect(detailCalls).toEqual([{ rewardCodeId: 7, redemptionId: 51 }])
  })

  it('retries a failed detail request through the same resource seam', async () => {
    let detailCalls = 0
    redemptionFakes.list.mockResolvedValue(viewPage([makeViewSummary()]))
    redemptionFakes.get.mockImplementation(async () => {
      detailCalls += 1
      if (detailCalls === 1) throw new Error('detail unavailable')
      return makeViewDetail()
    })
    const { user } = renderDialog()
    await screen.findByText('用户 #23')

    await user.click(screen.getByRole('button', { name: /详情/ }))
    expect(await screen.findByText('兑换详情加载失败。')).toBeVisible()
    await user.click(screen.getByRole('button', { name: '重试兑换详情' }))

    expect(await screen.findByText('Credits Transaction ID')).toBeVisible()
    expect(detailCalls).toBe(2)
  })

  it('distinguishes an initial resource error from empty and retries it in place', async () => {
    let calls = 0
    redemptionFakes.list.mockImplementation(async () => {
      calls += 1
      if (calls === 1) throw new Error('list unavailable')
      return viewPage([makeViewSummary()])
    })
    const { user } = renderDialog()

    expect(await screen.findByText('兑换记录加载失败，请重试。')).toBeVisible()
    expect(screen.queryByText('暂无兑换记录')).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: '重试加载兑换记录' }))

    expect(await screen.findByText('用户 #23')).toBeVisible()
    expect(calls).toBe(2)
  })

  it('preserves the last page when a background refresh fails and can retry it', async () => {
    let calls = 0
    redemptionFakes.list.mockImplementation(async () => {
      calls += 1
      if (calls === 2) throw new Error('refresh unavailable')
      return viewPage([makeViewSummary()])
    })
    const { user } = renderDialog()
    await screen.findByText('用户 #23')

    await user.click(screen.getByRole('button', { name: '刷新兑换记录' }))
    expect(await screen.findByText('刷新兑换记录失败，正在显示上次成功的数据。')).toBeVisible()
    expect(screen.getByText('用户 #23')).toBeVisible()
    await user.click(screen.getByRole('button', { name: '重试刷新兑换记录' }))

    await waitFor(() =>
      expect(
        screen.queryByText('刷新兑换记录失败，正在显示上次成功的数据。'),
      ).not.toBeInTheDocument(),
    )
    expect(calls).toBe(3)
  })

  it('pages with the next keyset cursor through the resource seam', async () => {
    const cursors: Array<number | undefined> = []
    redemptionFakes.list.mockImplementation(async (_id: number, input?: object) => {
      const beforeId = (input as { beforeId?: number }).beforeId
      cursors.push(beforeId)
      return beforeId === undefined
        ? viewPage([makeViewSummary()], 40)
        : viewPage([
            makeViewSummary({
              id: 39,
              userPublicId: '00000000-0000-0000-0000-000000000043',
              displayName: '用户 #43',
            }),
          ])
    })
    const { user } = renderDialog()
    await screen.findByText('用户 #23')

    await user.click(screen.getByRole('button', { name: '下一页' }))

    expect(await screen.findByText('用户 #43')).toBeVisible()
    expect(screen.getByText('2 / 2')).toBeVisible()
    expect(cursors).toEqual([undefined, 40])
  })

  it('shows a compact empty state for a RewardCode with no successful Redemption', async () => {
    redemptionFakes.list.mockResolvedValue(viewPage([]))
    renderDialog()

    expect(await screen.findByText('暂无兑换记录')).toBeVisible()
    expect(screen.queryByText(/用户成功兑换后/)).not.toBeInTheDocument()
  })

  it('clears cursor and expanded detail when switching to another RewardCode', async () => {
    const listCalls: Array<{ rewardCodeId: number; beforeId: number | undefined }> = []
    redemptionFakes.list.mockImplementation(async (rewardCodeId: number, input?: object) => {
      const beforeId = (input as { beforeId?: number }).beforeId
      listCalls.push({ rewardCodeId, beforeId })
      return viewPage([
        makeViewSummary({
          rewardCodeId,
          userPublicId:
            rewardCodeId === 7
              ? '00000000-0000-0000-0000-000000000023'
              : '00000000-0000-0000-0000-000000000088',
          displayName: rewardCodeId === 7 ? '用户 #23' : '用户 #88',
        }),
      ])
    })
    redemptionFakes.get.mockResolvedValue(makeViewDetail())
    const { rerender, user } = renderDialog()
    await screen.findByText('用户 #23')
    await user.click(screen.getByRole('button', { name: /详情/ }))
    expect(await screen.findByText('Credits Transaction ID')).toBeVisible()

    await rerender({
      visible: true,
      rewardCode: makeRewardCode({ id: 8, displayName: '八月奖励' }),
    })

    expect(await screen.findByText('用户 #88')).toBeVisible()
    expect(screen.queryByText('Credits Transaction ID')).not.toBeInTheDocument()
    expect(listCalls.at(-1)).toEqual({ rewardCodeId: 8, beforeId: undefined })
  })

  it('ignores a late page response after close and opens with clean state', async () => {
    let releaseFirstPage!: () => void
    const firstPageGate = new Promise<void>((resolve) => {
      releaseFirstPage = resolve
    })
    let calls = 0
    redemptionFakes.list.mockImplementation(async (rewardCodeId: number) => {
      calls += 1
      if (calls === 1) {
        await firstPageGate
        return viewPage([makeViewSummary()])
      }
      return viewPage([
        makeViewSummary({
          rewardCodeId,
          userPublicId: '00000000-0000-0000-0000-000000000088',
          displayName: '用户 #88',
        }),
      ])
    })
    const { rerender } = renderDialog()
    await waitFor(() => expect(calls).toBe(1))

    await rerender({ visible: false, rewardCode: makeRewardCode() })
    releaseFirstPage()
    await rerender({
      visible: true,
      rewardCode: makeRewardCode({ id: 8, displayName: '八月奖励' }),
    })

    expect(await screen.findByText('用户 #88')).toBeVisible()
    expect(screen.queryByText('用户 #23')).not.toBeInTheDocument()
  })

  it('ignores a late detail response after close and does not restore the old expansion', async () => {
    let releaseDetail!: () => void
    const detailGate = new Promise<void>((resolve) => {
      releaseDetail = resolve
    })
    redemptionFakes.list.mockResolvedValue(viewPage([makeViewSummary()]))
    redemptionFakes.get.mockImplementation(async () => {
      await detailGate
      return makeViewDetail()
    })
    const { rerender, user } = renderDialog()
    await screen.findByText('用户 #23')
    await user.click(screen.getByRole('button', { name: /详情/ }))
    expect(screen.getByLabelText('加载兑换详情')).toBeVisible()

    await rerender({ visible: false, rewardCode: makeRewardCode() })
    releaseDetail()
    await rerender({ visible: true, rewardCode: makeRewardCode() })

    expect(await screen.findByText('用户 #23')).toBeVisible()
    expect(screen.queryByText('Credits Transaction ID')).not.toBeInTheDocument()
  })
})

function renderDialog(overrides: Record<string, unknown> = {}) {
  // Fresh client per render: the list query caches by key, so sharing one
  // across tests would serve stale pages and hide resource calls.
  const queryClient = new QueryClient()
  return renderWithUser(RewardCodeRedemptionsDialog, {
    props: {
      visible: true,
      rewardCode: makeRewardCode(),
      ...overrides,
    },
    global: {
      plugins: [[VueQueryPlugin, { queryClient }]],
    },
  })
}

function viewPage(
  redemptions: Array<{
    id: number
    rewardCodeId: number
    userPublicId: string
    displayName: string
    creditedAmount: number
    redeemedAt: string
  }>,
  nextBeforeId?: number,
) {
  return {
    redemptions,
    ...(nextBeforeId === undefined ? {} : { nextBeforeId }),
  }
}

function makeRewardCode(overrides: Partial<RewardCode> = {}): RewardCode {
  return {
    id: 7,
    displayName: '新用户奖励',
    maskedCodeHint: '•••• 2026',
    creditAmount: 80,
    maxRedemptions: 100,
    redemptionCount: 12,
    enabled: true,
    status: 'enabled',
    createdAt: '2026-07-20T10:00:00Z',
    updatedAt: '2026-07-20T10:00:00Z',
    ...overrides,
  }
}

function makeViewSummary(
  overrides: Partial<{
    id: number
    rewardCodeId: number
    userPublicId: string
    displayName: string
    creditedAmount: number
    redeemedAt: string
  }> = {},
) {
  return {
    id: 51,
    rewardCodeId: 7,
    userPublicId: '00000000-0000-0000-0000-000000000023',
    displayName: '用户 #23',
    creditedAmount: 80,
    redeemedAt: '2026-07-22T03:04:05Z',
    ...overrides,
  }
}

function makeViewDetail() {
  return {
    ...makeViewSummary(),
    creditTransactionId: 'credit-transaction-9001-with-a-long-audit-reference',
    maskedCodeHint: '•••• 2026',
  }
}
