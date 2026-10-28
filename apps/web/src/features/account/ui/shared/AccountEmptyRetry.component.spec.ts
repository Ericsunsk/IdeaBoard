import { describe, expect, it } from 'vitest'
import { renderWithUser, screen } from '../../../../test/render'
import AccountEmptyRetry from './AccountEmptyRetry.vue'

describe('AccountEmptyRetry', () => {
  it('announces failures politely and emits retry', async () => {
    const { user, emitted } = renderWithUser(AccountEmptyRetry, {
      props: {
        title: 'No billing records yet',
        description: 'Your Ideas activity will appear here.',
        retryLabel: 'Reload',
      },
    })

    expect(screen.getByText('No billing records yet')).toBeVisible()
    expect(screen.getByText('Your Ideas activity will appear here.')).toBeVisible()
    const retry = screen.getByRole('button', { name: 'Reload' })
    expect(retry.parentElement).toHaveAttribute('aria-live', 'polite')
    await user.click(retry)
    expect(emitted().retry).toHaveLength(1)
  })

  it('renders the forbidden state as a neutral status without a live region', () => {
    renderWithUser(AccountEmptyRetry, {
      props: { title: 'Access restricted', retryLabel: 'Reload', announce: 'status' },
    })

    const status = screen.getByRole('status')
    expect(status).toHaveTextContent('Access restricted')
    expect(status).not.toHaveAttribute('aria-live')
    expect(screen.queryByRole('button', { name: 'Reload' })).toBeVisible()
  })
})
