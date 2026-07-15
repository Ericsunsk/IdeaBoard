import PrimeVue from 'primevue/config'
import { describe, expect, it, vi } from 'vitest'
import { primeVueTestOptions } from '../../../test/primeVue'
import { renderWithUser, screen } from '../../../test/render'
import UserRewardsPane from './UserRewardsPane.vue'

const redeemMock = vi.hoisted(() => vi.fn())
const rewardRedemptionSucceededMock = vi.hoisted(() => vi.fn())
const rewardRedemptionFailedMock = vi.hoisted(() => vi.fn())

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

function renderRewardsPane() {
  return renderWithUser(UserRewardsPane, {
    global: {
      plugins: [[PrimeVue, primeVueTestOptions]],
      stubs: {
        Gift: true,
      },
    },
  })
}

describe('UserRewardsPane', () => {
  it('renders the redeem form and empty history state', () => {
    renderRewardsPane()

    expect(screen.getByRole('heading', { name: '奖励中心' })).toBeVisible()
    expect(screen.getByRole('textbox', { name: '输入兑换码' })).toHaveAttribute(
      'placeholder',
      '请输入兑换码',
    )
    expect(screen.getByRole('button', { name: '兑换' })).toBeDisabled()
    expect(screen.getByRole('heading', { name: '兑换记录' })).toBeVisible()
    expect(screen.getByText('暂无兑换记录')).toBeVisible()
  })

  it('keeps the user-shell primary action visual contract', () => {
    renderRewardsPane()

    const redeemButton = screen.getByRole('button', { name: '兑换' })
    expect(redeemButton).not.toHaveClass('p-button')
    expect(redeemButton).toHaveClass(
      'bg-ib-creative-accent-hover',
      'text-ib-text-inverse',
    )
  })

  it('submits a trimmed redemption code through the local reward workflow', async () => {
    redeemMock.mockReset().mockResolvedValue({ credited_amount: 80, replayed: false })
    rewardRedemptionSucceededMock.mockReset()
    const { user } = renderRewardsPane()

    await user.type(screen.getByRole('textbox', { name: '输入兑换码' }), '  IDEA-2026  ')
    await user.click(screen.getByRole('button', { name: '兑换' }))

    expect(redeemMock).toHaveBeenCalledWith('IDEA-2026')
    expect(rewardRedemptionSucceededMock).toHaveBeenCalledWith(80, false)
    expect(screen.getByRole('textbox', { name: '输入兑换码' })).toHaveValue('')
  })

  it('retains the code when redemption fails', async () => {
    const error = new Error('network')
    redeemMock.mockReset().mockRejectedValue(error)
    rewardRedemptionFailedMock.mockReset()
    const { user } = renderRewardsPane()

    await user.type(screen.getByRole('textbox', { name: '输入兑换码' }), 'IDEA2026')
    await user.click(screen.getByRole('button', { name: '兑换' }))

    expect(rewardRedemptionFailedMock).toHaveBeenCalledWith(error)
    expect(screen.getByRole('textbox', { name: '输入兑换码' })).toHaveValue('IDEA2026')
  })
})
