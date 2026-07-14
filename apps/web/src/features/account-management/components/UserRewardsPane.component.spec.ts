import PrimeVue from 'primevue/config'
import { describe, expect, it } from 'vitest'
import { renderWithUser, screen } from '../../../test/render'
import UserRewardsPane from './UserRewardsPane.vue'

function renderRewardsPane() {
  return renderWithUser(UserRewardsPane, {
    global: {
      plugins: [[PrimeVue, { unstyled: true }]],
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

  it('submits a trimmed redemption code', async () => {
    const { emitted, user } = renderRewardsPane()

    await user.type(screen.getByRole('textbox', { name: '输入兑换码' }), '  IDEA-2026  ')
    await user.click(screen.getByRole('button', { name: '兑换' }))

    expect(emitted().redeem).toEqual([['IDEA-2026']])
  })
})
