import { describe, expect, it } from 'vitest'
import { renderWithUser, screen } from '../../../test/render'
import UserRechargePurchaseCard from './UserRechargePurchaseCard.vue'

describe('UserRechargePurchaseCard', () => {
  it('shows the Dispute restriction while keeping the account recharge surface readable', () => {
    renderWithUser(UserRechargePurchaseCard, {
      props: {
        options: {
          purchases_available: false,
          unavailable_reason: 'wallet_restricted',
          restriction_type: 'payment_dispute',
          quick_idea_quantities: [500, 1000],
          min_idea_quantity: 500,
          max_idea_quantity: 500000,
          ideas_per_usd: 100,
          terms: { version: 'v1', url: '/terms' },
          refund_policy: { version: 'v1', url: '/refund-policy' },
        },
        pending: false,
        error: null,
        checkoutPending: false,
        checkoutError: null,
      },
    })

    expect(screen.getByText('暂时无法充值')).toBeVisible()
    expect(screen.getByText(/银行卡 Dispute 处理中/)).toBeVisible()
  })
})
