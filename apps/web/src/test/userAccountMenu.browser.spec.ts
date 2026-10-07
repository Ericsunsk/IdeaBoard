import { describe, expect, it } from 'vitest'
import { page } from 'vitest/browser'
import { renderWithUser, screen, waitFor } from './render'
import { i18n } from '../shared/i18n'
import UserAccountMenu from '../shared/components/UserAccountMenu.vue'
import '../app/style.css'

describe('Support QR browser interaction', () => {
  it.each([
    [1280, 800],
    [390, 844],
    [320, 568],
  ])('keeps the QR usable at %ix%i', async (width, height) => {
    await page.viewport(width, height)
    i18n.global.locale.value = 'en'
    const { user } = renderWithUser(UserAccountMenu, {
      props: {
        items: [],
        supportChannels: [
          {
            id: 'discord',
            name: 'Discord',
            kind: 'discord',
            url: 'https://discord.gg/example',
            enabled: true,
            sort_order: 0,
          },
        ],
      },
    })
    const trigger = screen.getByRole('button', { name: 'Account menu' })
    trigger.style.position = 'fixed'
    trigger.style.top = '16px'
    trigger.style.right = '16px'
    trigger.style.width = '32px'
    trigger.style.height = '32px'
    await user.click(trigger)
    const support = await screen.findByRole('menuitem', { name: 'Support' })
    await user.click(support)
    const channel = await screen.findByRole('menuitem', { name: 'Discord' })
    await user.click(channel)
    const panel = await screen.findByRole('dialog', { name: 'Discord support QR code' })
    await user.hover(channel)
    await user.hover(panel)
    await new Promise((resolve) => setTimeout(resolve, 300))
    expect(screen.getByRole('menuitem', { name: 'Discord' })).toBeVisible()
    expect(screen.getByRole('dialog', { name: 'Discord support QR code' })).toBeVisible()
    await waitFor(() => {
      const rect = panel.getBoundingClientRect()
      expect(rect.width).toBeGreaterThan(176)
      expect(rect.left).toBeGreaterThanOrEqual(10)
      expect(rect.right).toBeLessThanOrEqual(window.innerWidth - 10)
      expect(rect.top).toBeGreaterThanOrEqual(10)
      expect(rect.bottom).toBeLessThanOrEqual(window.innerHeight - 10)
      if (width < 640) expect(rect.top).toBeGreaterThan(channel.getBoundingClientRect().bottom)
    })
    await page.screenshot({ path: `../../node_modules/.cache/support-qr-${width}.png` })
    await user.keyboard('{Enter}')
    const link = await screen.findByRole('link', { name: /Open Discord/ })
    await waitFor(() => expect(document.activeElement).toBe(link))
    expect(trigger.getAttribute('aria-expanded')).toBe('true')
    expect(link.querySelector('svg path')).not.toBeNull()
    await user.keyboard('{Escape}')
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
    await waitFor(() => expect(document.activeElement).toBe(channel))
    expect(trigger.getAttribute('aria-expanded')).toBe('true')
    await user.pointer([{ keys: '[TouchA>]', target: channel }, { keys: '[/TouchA]' }])
    channel.dispatchEvent(new PointerEvent('pointerleave', { pointerType: 'touch', bubbles: true }))
    await new Promise((resolve) => setTimeout(resolve, 300))
    expect(screen.getByRole('dialog')).toBeTruthy()
  })
})
