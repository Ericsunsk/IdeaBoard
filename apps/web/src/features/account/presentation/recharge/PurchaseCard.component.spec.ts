import { describe, expect, it } from 'vitest'
import { renderWithUser, screen } from '../../../../test/render'
import PurchaseCard from './PurchaseCard.vue'

describe('PurchaseCard', () => {
  it('shows the Dispute restriction while keeping the account recharge surface readable', () => {
    renderWithUser(PurchaseCard, {
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

    expect(screen.getByText('Top-up unavailable')).toBeVisible()
    expect(screen.getByText(/card dispute is in progress/i)).toBeVisible()
  })
})
