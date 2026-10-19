import { beforeEach, describe, expect, test, vi } from 'vitest'
import { setUserLocale } from '../../../../../core/i18n'
import { renderWithUser, screen } from '../../../../../test/render'
import SpotlightBanner from './SpotlightBanner.vue'

function renderBanner() {
  return renderWithUser(SpotlightBanner, {
    props: { spotlighterName: 'Bob', spotlighterColor: 'hsl(10 70% 45%)' },
  })
}

beforeEach(async () => {
  await setUserLocale('en')
})

describe('SpotlightBanner', () => {
  test('announces the invite as an alertdialog with accept and ignore', async () => {
    const view = renderBanner()
    const dialog = screen.getByRole('alertdialog')
    expect(dialog).toBeVisible()
    expect(dialog).toHaveAccessibleName('Bob started a spotlight')
    expect(screen.getByText('Follow Bob’s camera to stay in sync?')).toBeVisible()

    await view.user.click(screen.getByRole('button', { name: 'Follow' }))
    expect(view.emitted()['accept']).toHaveLength(1)

    await view.user.click(screen.getByRole('button', { name: 'Dismiss' }))
    expect(view.emitted()['ignore']).toHaveLength(1)
  })

  test('moves focus to the accept control on mount', () => {
    // happy-dom resets document focus after mount, so assert the focus
    // attempt itself: production browsers keep the landed focus.
    const focused: Element[] = []
    const focusSpy = vi.spyOn(HTMLElement.prototype, 'focus').mockImplementation(function (
      this: HTMLElement,
    ) {
      focused.push(this)
    })
    try {
      renderBanner()
      expect(focused).toContain(screen.getByRole('button', { name: 'Follow' }))
    } finally {
      focusSpy.mockRestore()
    }
  })

  test('ignores the invite on Escape from within the banner', async () => {
    const view = renderBanner()
    ;(screen.getByRole('button', { name: 'Follow' }) as HTMLButtonElement).focus()
    await view.user.keyboard('{Escape}')
    expect(view.emitted()['ignore']).toHaveLength(1)
  })
})
