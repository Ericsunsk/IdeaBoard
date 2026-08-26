import PrimeVue from 'primevue/config'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { userPrimeVueTestOptions } from '../../../test/primeVue'
import { renderWithUser, screen } from '../../../test/render'
import UserRewardsPane from './UserRewardsPane.vue'

const redeemMock = vi.hoisted(() => vi.fn())
const rewardRedemptionSucceededMock = vi.hoisted(() => vi.fn())
const rewardRedemptionFailedMock = vi.hoisted(() => vi.fn())
const retryInitialMock = vi.hoisted(() => vi.fn())
const loadMoreMock = vi.hoisted(() => vi.fn())
const historyMock = vi.hoisted(() => ({
  redemptions: [] as Array<{
    id: number
    reward_code_display_name: string
    credited_amount: number
    masked_code_hint: string
    redeemed_at: string
  }>,
  isInitialPending: false,
  initialError: null as Error | null,
  loadMoreError: null as Error | null,
  hasNextPage: false,
  isFetchingNextPage: false,
}))

vi.mock('../useUserRewardRedemption', async () => {
  const { ref } = await import('vue')
  return {
    useUserRewardRedemption: () => ({ pending: ref(false), redeem: redeemMock }),
  }
})

vi.mock('../useProfileFeedback', () => ({
  useProfileFeedback: () => ({
    rewardRedemptionSucceeded: rewardRedemptionSucceededMock,
    rewardRedemptionFailed: rewardRedemptionFailedMock,
  }),
}))

vi.mock('../useUserRewardRedemptions', async () => {
  const { reactive, toRefs } = await import('vue')
  const state = reactive(historyMock)
  return {
    useUserRewardRedemptions: () => ({
      ...toRefs(state),
      retryInitial: retryInitialMock,
      loadMore: loadMoreMock,
    }),
  }
})

function renderRewardsPane() {
  return renderWithUser(UserRewardsPane, {
    global: {
      plugins: [[PrimeVue, userPrimeVueTestOptions]],
      stubs: {
        Gift: true,
      },
    },
  })
}

describe('UserRewardsPane', () => {
  beforeEach(() => {
    historyMock.redemptions = []
    historyMock.isInitialPending = false
    historyMock.initialError = null
    historyMock.loadMoreError = null
    historyMock.hasNextPage = false
    historyMock.isFetchingNextPage = false
    retryInitialMock.mockReset()
    loadMoreMock.mockReset()
  })

  it('renders the redeem form and empty history state', () => {
    renderRewardsPane()

    expect(screen.getByRole('heading', { name: 'Redeem code' })).toBeVisible()
    expect(screen.getByRole('textbox', { name: 'Enter redemption code' })).toHaveAttribute(
      'placeholder',
      'Enter redemption code',
    )
    expect(screen.getByRole('button', { name: 'Redeem' })).toBeDisabled()
    expect(screen.getByRole('heading', { name: 'Redemption history' })).toBeVisible()
    expect(screen.getByText('No redemptions yet')).toBeVisible()
  })

  it('keeps the user-shell primary action visual contract', () => {
    renderRewardsPane()

    const redeemButton = screen.getByRole('button', { name: 'Redeem' })
    expect(redeemButton).not.toHaveClass('p-button')
    expect(redeemButton).toHaveClass('bg-ib-creative-accent-hover', 'text-ib-text-inverse')
  })

  it('submits a trimmed redemption code through the local reward workflow', async () => {
    redeemMock.mockReset().mockResolvedValue({ credited_amount: 80, replayed: false })
    rewardRedemptionSucceededMock.mockReset()
    const { user } = renderRewardsPane()

    await user.type(screen.getByRole('textbox', { name: 'Enter redemption code' }), '  IDEA-2026  ')
    await user.click(screen.getByRole('button', { name: 'Redeem' }))

    expect(redeemMock).toHaveBeenCalledWith('IDEA-2026')
    expect(rewardRedemptionSucceededMock).toHaveBeenCalledWith(80, false)
    expect(screen.getByRole('textbox', { name: 'Enter redemption code' })).toHaveValue('')
  })

  it('retains the code when redemption fails', async () => {
    const error = new Error('network')
    redeemMock.mockReset().mockRejectedValue(error)
    rewardRedemptionFailedMock.mockReset()
    const { user } = renderRewardsPane()

    await user.type(screen.getByRole('textbox', { name: 'Enter redemption code' }), 'IDEA2026')
    await user.click(screen.getByRole('button', { name: 'Redeem' }))

    expect(rewardRedemptionFailedMock).toHaveBeenCalledWith(error)
    expect(screen.getByRole('textbox', { name: 'Enter redemption code' })).toHaveValue('IDEA2026')
  })

  it('shows paginated redemption details with masked codes and Shanghai time', () => {
    historyMock.redemptions = [
      {
        id: 41,
        reward_code_display_name: '夏日灵感奖励',
        credited_amount: 80,
        masked_code_hint: '•••• 2026',
        redeemed_at: '2026-07-15T10:00:00Z',
      },
      {
        id: 40,
        reward_code_display_name: '内测奖励',
        credited_amount: 1,
        masked_code_hint: '•••• 0001',
        redeemed_at: '2026-07-14T16:05:06Z',
      },
    ]
    historyMock.hasNextPage = true
    renderRewardsPane()

    expect(screen.getByRole('table', { name: 'Redemption history table' })).toBeVisible()
    expect(screen.getByText('夏日灵感奖励')).toBeVisible()
    expect(screen.getByText('+80 Ideas')).toBeVisible()
    expect(screen.getByText('+1 Idea')).toBeVisible()
    expect(screen.getByText('•••• 2026')).toBeVisible()
    expect(screen.queryByText('IDEA-2026')).not.toBeInTheDocument()
    expect(screen.getByText('07/15/2026, 18:00:00')).toBeVisible()
    expect(screen.getByText('07/15/2026, 00:05:06')).toBeVisible()
    expect(screen.getByRole('button', { name: 'Load more' })).toBeVisible()
  })

  it('provides recovery actions for initial and load-more errors', async () => {
    historyMock.initialError = new Error('奖励服务暂不可用')
    const { unmount, user } = renderRewardsPane()

    expect(screen.getByRole('alert')).toHaveTextContent('Network error. Please try again.')
    await user.click(screen.getByRole('button', { name: 'Reload' }))
    expect(retryInitialMock).toHaveBeenCalledOnce()

    unmount()
    historyMock.initialError = null
    historyMock.redemptions = [
      {
        id: 41,
        reward_code_display_name: '夏日灵感奖励',
        credited_amount: 80,
        masked_code_hint: '•••• 2026',
        redeemed_at: '2026-07-15T10:00:00Z',
      },
    ]
    historyMock.loadMoreError = new Error('下一页加载失败')
    const next = renderRewardsPane()

    expect(screen.getByRole('alert')).toHaveTextContent('Network error. Please try again.')
    await next.user.click(screen.getByRole('button', { name: 'Retry load' }))
    expect(loadMoreMock).toHaveBeenCalledOnce()
  })

  it('announces initial loading without showing a false empty state', () => {
    historyMock.isInitialPending = true
    renderRewardsPane()

    expect(screen.getByRole('status', { name: 'Loading redemption history…' })).toBeInTheDocument()
    expect(screen.queryByText('No redemptions yet')).not.toBeInTheDocument()
  })
})
