import { describe, expect, it, vi } from 'vitest'
import { renderWithUser, screen, within } from '../../test/render'
import LoadErrorPanel from './LoadErrorPanel.vue'

describe('LoadErrorPanel', () => {
  it('renders the destructive panel with message and a small retry button', async () => {
    const onRetry = vi.fn()
    renderWithUser(LoadErrorPanel, {
      props: { message: 'Load failed', retryLabel: 'Retry', onRetry },
    })

    const alert = screen.getByRole('alert')
    expect(alert).toHaveClass(
      'rounded-panel',
      'border',
      'border-destructive/30',
      'bg-destructive/10',
      'p-4',
    )
    expect(alert).toHaveClass('items-start', 'text-left')
    expect(screen.getByText('Load failed')).toBeVisible()
    expect(screen.queryByText('detail')).not.toBeInTheDocument()

    const retry = within(alert).getByRole('button', { name: 'Retry' })
    expect(retry).toHaveClass('h-8', 'px-3', 'text-xs')
    expect(retry).toHaveClass('bg-primary')
  })

  it('emits retry when the button is clicked', async () => {
    const onRetry = vi.fn()
    const { user } = renderWithUser(LoadErrorPanel, {
      props: { message: 'Load failed', retryLabel: 'Retry', onRetry },
    })

    await user.click(screen.getByRole('button', { name: 'Retry' }))
    expect(onRetry).toHaveBeenCalledOnce()
  })

  it('renders the detail copy and centers content with the ghost retry treatment', async () => {
    const { user } = renderWithUser(LoadErrorPanel, {
      props: {
        message: 'Load failed',
        detail: 'Network error. Please try again.',
        retryLabel: 'Reload',
        align: 'center',
        retryVariant: 'ghost',
        onRetry: () => {},
      },
    })

    const alert = screen.getByRole('alert')
    expect(alert).toHaveClass('items-center', 'text-center')
    expect(screen.getByText('Network error. Please try again.')).toBeVisible()
    const retry = within(alert).getByRole('button', { name: 'Reload' })
    expect(retry).toHaveClass('hover:bg-accent')
    expect(retry).not.toHaveClass('bg-primary')
    await user.click(retry)
  })

  it('marks the retry button busy while retrying', () => {
    renderWithUser(LoadErrorPanel, {
      props: { message: 'Load failed', retryLabel: 'Retry', isRetrying: true, onRetry: () => {} },
    })

    const retry = screen.getByRole('button', { name: 'Retry' })
    expect(retry).toHaveAttribute('aria-busy', 'true')
    expect(retry).toBeDisabled()
  })
})
