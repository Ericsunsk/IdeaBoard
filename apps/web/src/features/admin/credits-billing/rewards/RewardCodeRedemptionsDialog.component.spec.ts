import PrimeVue from 'primevue/config'
import { describe, expect, it } from 'vitest'
import { http, HttpResponse, useMswHandlers } from '../../../../test/msw'
import { primeVueTestOptions } from '../../../../test/primeVue'
import { renderWithUser, screen, waitFor } from '../../../../test/render'
import RewardCodeRedemptionsDialog from './RewardCodeRedemptionsDialog.vue'
import type { RewardCode } from './rewardCodeAdminResource'

describe('RewardCodeRedemptionsDialog', () => {
  it('loads a dense Redemption page through the real feature resource HTTP seam', async () => {
    const requests: Array<{ rewardCodeId: string; beforeId: string | null; limit: string | null }> =
      []
    useMswHandlers(
      http.get('/api/admin/rewards/codes/:rewardCodeId/redemptions', ({ params, request }) => {
        const query = new URL(request.url).searchParams
        requests.push({
          rewardCodeId: String(params.rewardCodeId),
          beforeId: query.get('before_id'),
          limit: query.get('limit'),
        })
        return redemptionPageResponse([makeRedemptionSummary()])
      }),
    )

    renderDialog()

    expect(await screen.findByText('用户 #23')).toBeVisible()
    expect(screen.getByText('80 Ideas')).toBeVisible()
    expect(screen.getByText(/2026/)).toBeVisible()
    expect(screen.queryByText('Redemption ID')).not.toBeInTheDocument()
    expect(screen.queryByText('masked Code hint')).not.toBeInTheDocument()
    expect(requests).toEqual([{ rewardCodeId: '7', beforeId: null, limit: '20' }])
  })

  it('loads long technical detail only after the row is expanded', async () => {
    let releaseDetail!: () => void
    const detailGate = new Promise<void>((resolve) => {
      releaseDetail = resolve
    })
    const detailRequests: Array<{ rewardCodeId: string; redemptionId: string }> = []
    useMswHandlers(
      listHandler(),
      http.get(
        '/api/admin/rewards/codes/:rewardCodeId/redemptions/:redemptionId',
        async ({ params }) => {
          detailRequests.push({
            rewardCodeId: String(params.rewardCodeId),
            redemptionId: String(params.redemptionId),
          })
          await detailGate
          return redemptionDetailResponse()
        },
      ),
    )
    const { user } = renderDialog()
    await screen.findByText('用户 #23')

    expect(detailRequests).toEqual([])
    await user.click(screen.getByRole('button', { name: /Row/ }))
    expect(screen.getByLabelText('加载兑换详情')).toBeVisible()
    releaseDetail()

    expect(await screen.findByText('Redemption ID')).toBeVisible()
    expect(screen.getByText('Credits Transaction ID')).toBeVisible()
    expect(screen.getByText('masked Code hint')).toBeVisible()
    expect(screen.getByText('credit-transaction-9001-with-a-long-audit-reference')).toBeVisible()
    expect(detailRequests).toEqual([{ rewardCodeId: '7', redemptionId: '51' }])
  })

  it('retries a failed detail request through the same HTTP seam', async () => {
    let detailRequests = 0
    useMswHandlers(
      listHandler(),
      http.get('/api/admin/rewards/codes/:rewardCodeId/redemptions/:redemptionId', () => {
        detailRequests += 1
        if (detailRequests === 1) {
          return HttpResponse.json({ code: 503, msg: 'detail unavailable' }, { status: 503 })
        }
        return redemptionDetailResponse()
      }),
    )
    const { user } = renderDialog()
    await screen.findByText('用户 #23')

    await user.click(screen.getByRole('button', { name: /Row/ }))
    expect(await screen.findByText('兑换详情加载失败。')).toBeVisible()
    await user.click(screen.getByRole('button', { name: '重试兑换详情' }))

    expect(await screen.findByText('Credits Transaction ID')).toBeVisible()
    expect(detailRequests).toBe(2)
  })

  it('distinguishes an initial HTTP error from empty and retries it in place', async () => {
    let requests = 0
    useMswHandlers(
      http.get('/api/admin/rewards/codes/:rewardCodeId/redemptions', () => {
        requests += 1
        if (requests === 1) {
          return HttpResponse.json({ code: 503, msg: 'list unavailable' }, { status: 503 })
        }
        return redemptionPageResponse([makeRedemptionSummary()])
      }),
    )
    const { user } = renderDialog()

    expect(await screen.findByText('兑换记录加载失败，请重试。')).toBeVisible()
    expect(screen.queryByText('暂无兑换记录')).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: '重试加载兑换记录' }))

    expect(await screen.findByText('用户 #23')).toBeVisible()
    expect(requests).toBe(2)
  })

  it('preserves the last page when a background refresh fails and can retry it', async () => {
    let requests = 0
    useMswHandlers(
      http.get('/api/admin/rewards/codes/:rewardCodeId/redemptions', () => {
        requests += 1
        if (requests === 2) {
          return HttpResponse.json({ code: 503, msg: 'refresh unavailable' }, { status: 503 })
        }
        return redemptionPageResponse([makeRedemptionSummary()])
      }),
    )
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
    expect(requests).toBe(3)
  })

  it('pages with the next keyset cursor through the generated client URL', async () => {
    const cursors: Array<string | null> = []
    useMswHandlers(
      http.get('/api/admin/rewards/codes/:rewardCodeId/redemptions', ({ request }) => {
        const beforeId = new URL(request.url).searchParams.get('before_id')
        cursors.push(beforeId)
        return beforeId === null
          ? redemptionPageResponse([makeRedemptionSummary()], 40)
          : redemptionPageResponse([
              makeRedemptionSummary({
                id: 39,
                user_public_id: '00000000-0000-0000-0000-000000000043',
                display_name: '用户 #43',
              }),
            ])
      }),
    )
    const { user } = renderDialog()
    await screen.findByText('用户 #23')

    await user.click(screen.getByRole('button', { name: '下一页' }))

    expect(await screen.findByText('用户 #43')).toBeVisible()
    expect(screen.getByText('2 / 2')).toBeVisible()
    expect(cursors).toEqual([null, '40'])
  })

  it('shows a compact empty state for a RewardCode with no successful Redemption', async () => {
    useMswHandlers(listHandler([]))
    renderDialog()

    expect(await screen.findByText('暂无兑换记录')).toBeVisible()
    expect(screen.queryByText(/用户成功兑换后/)).not.toBeInTheDocument()
  })

  it('clears cursor and expanded detail when switching to another RewardCode', async () => {
    const listRequests: Array<{ rewardCodeId: string; beforeId: string | null }> = []
    useMswHandlers(
      http.get('/api/admin/rewards/codes/:rewardCodeId/redemptions', ({ params, request }) => {
        const rewardCodeId = String(params.rewardCodeId)
        listRequests.push({
          rewardCodeId,
          beforeId: new URL(request.url).searchParams.get('before_id'),
        })
        return redemptionPageResponse(
          [
            makeRedemptionSummary({
              reward_code_id: Number(rewardCodeId),
              user_public_id:
                rewardCodeId === '7'
                  ? '00000000-0000-0000-0000-000000000023'
                  : '00000000-0000-0000-0000-000000000088',
              display_name: rewardCodeId === '7' ? '用户 #23' : '用户 #88',
            }),
          ],
          rewardCodeId === '7' ? 40 : undefined,
        )
      }),
      http.get('/api/admin/rewards/codes/:rewardCodeId/redemptions/:redemptionId', () =>
        redemptionDetailResponse(),
      ),
    )
    const { rerender, user } = renderDialog()
    await screen.findByText('用户 #23')
    await user.click(screen.getByRole('button', { name: /Row/ }))
    expect(await screen.findByText('Credits Transaction ID')).toBeVisible()

    await rerender({
      visible: true,
      rewardCode: makeRewardCode({ id: 8, displayName: '八月奖励' }),
    })

    expect(await screen.findByText('用户 #88')).toBeVisible()
    expect(screen.queryByText('Credits Transaction ID')).not.toBeInTheDocument()
    expect(listRequests.at(-1)).toEqual({ rewardCodeId: '8', beforeId: null })
  })

  it('ignores a late page response after close and opens with clean state', async () => {
    let releaseFirstPage!: () => void
    const firstPageGate = new Promise<void>((resolve) => {
      releaseFirstPage = resolve
    })
    let requests = 0
    useMswHandlers(
      http.get('/api/admin/rewards/codes/:rewardCodeId/redemptions', async ({ params }) => {
        requests += 1
        if (requests === 1) {
          await firstPageGate
          return redemptionPageResponse([makeRedemptionSummary()])
        }
        return redemptionPageResponse([
          makeRedemptionSummary({
            reward_code_id: Number(params.rewardCodeId),
            user_public_id: '00000000-0000-0000-0000-000000000088',
            display_name: '用户 #88',
          }),
        ])
      }),
    )
    const { rerender } = renderDialog()
    await waitFor(() => expect(requests).toBe(1))

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
    useMswHandlers(
      listHandler(),
      http.get('/api/admin/rewards/codes/:rewardCodeId/redemptions/:redemptionId', async () => {
        await detailGate
        return redemptionDetailResponse()
      }),
    )
    const { rerender, user } = renderDialog()
    await screen.findByText('用户 #23')
    await user.click(screen.getByRole('button', { name: /Row/ }))
    expect(screen.getByLabelText('加载兑换详情')).toBeVisible()

    await rerender({ visible: false, rewardCode: makeRewardCode() })
    releaseDetail()
    await rerender({ visible: true, rewardCode: makeRewardCode() })

    expect(await screen.findByText('用户 #23')).toBeVisible()
    expect(screen.queryByText('Credits Transaction ID')).not.toBeInTheDocument()
  })
})

function renderDialog(overrides: Record<string, unknown> = {}) {
  return renderWithUser(RewardCodeRedemptionsDialog, {
    props: {
      visible: true,
      rewardCode: makeRewardCode(),
      ...overrides,
    },
    global: { plugins: [[PrimeVue, primeVueTestOptions]] },
  })
}

function listHandler(redemptions = [makeRedemptionSummary()]) {
  return http.get('/api/admin/rewards/codes/:rewardCodeId/redemptions', () =>
    redemptionPageResponse(redemptions),
  )
}

function redemptionPageResponse(
  redemptions: Array<Record<string, unknown>>,
  nextBeforeId?: number,
) {
  return HttpResponse.json({
    code: 0,
    msg: 'ok',
    data: {
      redemptions,
      ...(nextBeforeId === undefined ? {} : { next_before_id: nextBeforeId }),
    },
  })
}

function redemptionDetailResponse() {
  return HttpResponse.json({ code: 0, msg: 'ok', data: makeRedemptionDetail() })
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

function makeRedemptionSummary(overrides: Record<string, unknown> = {}) {
  return {
    id: 51,
    reward_code_id: 7,
    user_public_id: '00000000-0000-0000-0000-000000000023',
    display_name: '用户 #23',
    credited_amount: 80,
    redeemed_at: '2026-07-22T03:04:05Z',
    ...overrides,
  }
}

function makeRedemptionDetail(overrides: Record<string, unknown> = {}) {
  return {
    ...makeRedemptionSummary(),
    credit_transaction_id: 'credit-transaction-9001-with-a-long-audit-reference',
    masked_code_hint: '•••• 2026',
    ...overrides,
  }
}
