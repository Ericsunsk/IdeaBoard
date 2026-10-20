import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup } from '@testing-library/vue'
import { defineComponent, h } from 'vue'
import { useI18n } from 'vue-i18n'
import { renderWithUser, screen } from '../../../test/render'
import LoginAuthPanel from './LoginAuthPanel.vue'
import RegisterAuthPanel from './RegisterAuthPanel.vue'
import '../../../app/style.css'

// Keep IO outside this layout and hit-testing contract.
vi.mock('./panelControls', () => ({
  useAuthPanelControls: () => ({ t: useI18n().t, userStore: {}, authFeedback: {} }),
}))
vi.mock('../session/transport', () => ({
  authTransport: { getCapabilities: async () => ({ social: {}, providers: [] }) },
}))

afterEach(cleanup)

describe.each([
  ['login', LoginAuthPanel],
  ['register', RegisterAuthPanel],
] as const)('%s close action', (_name, panel) => {
  it.each([320, 390])('receives clicks throughout its target at %spx width', (width) => {
    const close = vi.fn()
    renderWithUser(
      defineComponent({
        setup: () => () =>
          h('div', { style: { width: `${width}px` } }, h(panel, { onClose: close })),
      }),
      { global: { stubs: { RouterLink: { template: '<a><slot /></a>' } } } },
    )
    const button = screen.getByRole('button', { name: 'Close' })
    const bounds = button.getBoundingClientRect()
    for (const x of [0.2, 0.5, 0.8]) {
      for (const y of [0.2, 0.5, 0.8]) {
        const target = document.elementFromPoint(
          bounds.left + bounds.width * x,
          bounds.top + bounds.height * y,
        )
        expect(target === button || button.contains(target)).toBe(true)
        if (target instanceof HTMLElement) target.click()
        else target?.dispatchEvent(new MouseEvent('click', { bubbles: true }))
      }
    }
    expect(close).toHaveBeenCalledTimes(9)
  })
})
