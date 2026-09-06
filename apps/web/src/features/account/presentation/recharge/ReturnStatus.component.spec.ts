import { afterEach, describe, expect, it, vi } from 'vitest'
import { renderWithUser, screen } from '../../../../test/render'
import ReturnStatus from './ReturnStatus.vue'

describe('ReturnStatus', () => {
  afterEach(() => vi.useRealTimers())

  it('distinguishes normal confirmation from a payment delayed beyond 60 seconds', async () => {
    vi.useFakeTimers()
    renderWithUser(ReturnStatus, {
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

    expect(screen.getByText('Confirming your payment result')).toBeVisible()
    expect(screen.queryByText('Processing Ideas')).not.toBeInTheDocument()

    await vi.advanceTimersByTimeAsync(60_000)

    expect(screen.getByText('Processing Ideas')).toBeVisible()
    expect(screen.getByText(/No need to pay again/)).toBeVisible()
  })

  it('treats a cancelled Checkout return as navigation context until backend credit is confirmed', async () => {
    const view = renderWithUser(ReturnStatus, {
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

    expect(screen.getByText('Back on IdeaBoard')).toBeVisible()
    expect(screen.getByText(/has not been completed/)).toBeVisible()

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

    expect(screen.getByText('1,000 Ideas added')).toBeVisible()
  })
})
