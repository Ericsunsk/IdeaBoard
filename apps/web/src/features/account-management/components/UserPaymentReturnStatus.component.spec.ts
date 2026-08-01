import { afterEach, describe, expect, it, vi } from 'vitest'
import { renderWithUser, screen } from '../../../test/render'
import UserPaymentReturnStatus from './UserPaymentReturnStatus.vue'

describe('UserPaymentReturnStatus', () => {
  afterEach(() => vi.useRealTimers())

  it('distinguishes normal confirmation from a payment delayed beyond 60 seconds', async () => {
    vi.useFakeTimers()
    renderWithUser(UserPaymentReturnStatus, {
      props: {
        pending: false,
        error: null,
        source: 'success',
        status: {
          idea_quantity: 1000,
          status: 'processing',
          updated_at: '2026-08-01T00:30:00Z',
        },
      },
    })

    expect(screen.getByText('正在确认支付结果')).toBeVisible()
    expect(screen.queryByText('到账处理中')).not.toBeInTheDocument()

    await vi.advanceTimersByTimeAsync(60_000)

    expect(screen.getByText('到账处理中')).toBeVisible()
    expect(screen.getByText(/无需重复支付/)).toBeVisible()
  })

  it('treats a cancelled Checkout return as navigation context until backend credit is confirmed', async () => {
    const view = renderWithUser(UserPaymentReturnStatus, {
      props: {
        pending: false,
        error: null,
        source: 'cancel',
        status: {
          idea_quantity: 1000,
          status: 'processing',
          updated_at: '2026-08-01T00:30:00Z',
        },
      },
    })

    expect(screen.getByText('已返回 IdeaBoard')).toBeVisible()
    expect(screen.getByText(/订单未到账/)).toBeVisible()

    await view.rerender({
      pending: false,
      error: null,
      source: 'cancel',
      status: {
        idea_quantity: 1000,
        status: 'credited',
        updated_at: '2026-08-01T00:31:00Z',
      },
    })

    expect(screen.getByText('1,000 Ideas 已到账')).toBeVisible()
  })
})
