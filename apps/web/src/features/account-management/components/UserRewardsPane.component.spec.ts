import PrimeVue from 'primevue/config'
import { describe, expect, it, vi } from 'vitest'
import { primeVueTestOptions } from '../../../test/primeVue'
import { renderWithUser, screen } from '../../../test/render'
import UserRewardsPane from './UserRewardsPane.vue'

const rewardRedemptionUnavailableMock = vi.hoisted(() => vi.fn())

vi.mock('../useProfileFeedback', () => ({
  useProfileFeedback: () => ({
    rewardRedemptionUnavailable: rewardRedemptionUnavailableMock,
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
    rewardRedemptionUnavailableMock.mockReset()
    const { user } = renderRewardsPane()

    await user.type(screen.getByRole('textbox', { name: '输入兑换码' }), '  IDEA-2026  ')
    await user.click(screen.getByRole('button', { name: '兑换' }))

    expect(rewardRedemptionUnavailableMock).toHaveBeenCalledOnce()
  })
})
