import '@testing-library/jest-dom/vitest'

import { page, commands, userEvent as browserUser } from 'vitest/browser'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, h, ref, Transition, nextTick } from 'vue'
import { renderWithUser, screen, waitFor } from './render'
import { cleanup } from '@testing-library/vue'
import { ibFormInputClass } from '../theme/formControls'
import UserAccountMenu from '../features/account/ui/chrome/UserAccountMenu.vue'
import { useTheme } from '../theme/composables/useTheme'
import { i18n } from '../core/i18n'
import '../app/style.css'
import { Button } from '../components/ui/button'
import { Input } from '../components/ui/input'
import ActionButton from '../core/components/ActionButton.vue'
import UserConfirmDialog from '../core/components/UserConfirmDialog.vue'
import {
  activeAdminConfirm,
  useAdminConfirm,
} from '../features/admin/shared/session/feedback/useAdminConfirm'
import AdminConfirmDialog from '../features/admin/shared/ui/feedback/AdminConfirmDialog.vue'
import { dialogMotionTransitionName } from '../core/motion/dialogMotion'

declare module 'vitest/internal/browser' {
  interface BrowserCommands {
    emulateMotion: (reducedMotion: 'reduce' | 'no-preference') => Promise<void>
  }
}

function styleOf(element: Element): CSSStyleDeclaration {
  return window.getComputedStyle(element)
}

function assertMotionTokens(element: Element, isReduced: boolean) {
  const style = styleOf(element)
  expect(style.transitionDuration).toBe(isReduced ? '0s' : '0.16s')
  expect(style.transitionTimingFunction).toContain('cubic-bezier(0.2, 0.8, 0.2, 1)')
}

async function setMotion(isReduced: boolean) {
  await commands.emulateMotion(isReduced ? 'reduce' : 'no-preference')
}

describe('P1 theme, control state and focus browser contracts', () => {
  afterEach(async () => {
    cleanup()
    activeAdminConfirm.value?.cancel()
    useTheme().setTheme('light')
    await setMotion(false)
  })

  beforeEach(async () => {
    document.documentElement.classList.remove('dark')
    i18n.global.locale.value = 'en'
    activeAdminConfirm.value = null
    await setMotion(false)
  })

  it('applies semantic motion policy and reduced-motion computed values', async () => {
    const host = document.createElement('button')
    host.className = 'control-state'
    document.body.append(host)
    const normal = styleOf(host)
    expect(normal.getPropertyValue('--motion-control-duration').trim()).toBe('160ms')
    expect(normal.getPropertyValue('--motion-enter-duration').trim()).toBe('280ms')
    expect(normal.getPropertyValue('--motion-exit-duration').trim()).toBe('280ms')
    expect(normal.getPropertyValue('--motion-feedback-duration').trim()).toBe('160ms')
    for (const intent of ['control', 'feedback']) {
      expect(normal.getPropertyValue(`--motion-${intent}-easing`).trim()).toBe(
        'cubic-bezier(0.2, 0.8, 0.2, 1)',
      )
    }
    for (const intent of ['enter', 'exit']) {
      expect(normal.getPropertyValue(`--motion-${intent}-easing`).trim()).toBe(
        'cubic-bezier(0.2, 0, 0.2, 1)',
      )
    }
    // The built CSS must not recreate deprecated aliases in the runtime cascade.
    const properties = [...normal].filter((property) => property.startsWith('--'))
    expect(properties.filter((property) => property.startsWith('--duration-ib-'))).toEqual([])
    expect(properties.filter((property) => property.startsWith('--ease-ib-'))).toEqual([])
    expect(normal.transitionDuration).toBe('0.16s')
    expect(normal.transitionTimingFunction).toBe('cubic-bezier(0.2, 0.8, 0.2, 1)')
    await setMotion(true)
    expect(matchMedia('(prefers-reduced-motion: reduce)').matches).toBe(true)
    const reduced = styleOf(host)
    expect(reduced.getPropertyValue('--motion-control-duration').trim()).toBe('0ms')
    expect(reduced.getPropertyValue('--motion-enter-duration').trim()).toBe('0ms')
    expect(reduced.getPropertyValue('--motion-exit-duration').trim()).toBe('0ms')
    expect(reduced.getPropertyValue('--motion-feedback-duration').trim()).toBe('0ms')
    assertMotionTokens(host, true)
    host.remove()
  })

  it('applies actual dialog transition duration, easing and transform policy', async () => {
    const isShown = ref(false)
    let entered: { duration: string; easing: string } | undefined
    let left: { duration: string; easing: string } | undefined
    const Fixture = defineComponent({
      setup: () => () =>
        h(
          Transition,
          {
            name: dialogMotionTransitionName,
            onEnter: (element: Element) => {
              entered = {
                duration: styleOf(element).transitionDuration,
                easing: styleOf(element).transitionTimingFunction,
              }
            },
            onLeave: (element: Element) => {
              left = {
                duration: styleOf(element).transitionDuration,
                easing: styleOf(element).transitionTimingFunction,
              }
            },
          },
          () =>
            isShown.value
              ? h(
                  'div',
                  {
                    'data-testid': 'motion-dialog',
                    style: { '--dialog-motion-from-transform': 'translateY(8px)' },
                  },
                  'Motion dialog',
                )
              : null,
        ),
    })
    renderWithUser(Fixture, { global: { stubs: { transition: false } } })
    isShown.value = true
    await nextTick()
    const dialog = screen.getByTestId('motion-dialog')
    expect(entered?.duration).toBe('0.28s, 0.28s')
    expect(entered?.easing).toBe('cubic-bezier(0.2, 0, 0.2, 1), cubic-bezier(0.2, 0, 0.2, 1)')
    expect(styleOf(dialog).transform).toBe('matrix(1, 0, 0, 1, 0, 8)')
    isShown.value = false
    await nextTick()
    expect(left?.duration).toBe('0.28s, 0.28s')
    expect(left?.easing).toBe('cubic-bezier(0.2, 0, 0.2, 1), cubic-bezier(0.2, 0, 0.2, 1)')
    await waitFor(() => expect(screen.queryByTestId('motion-dialog')).toBeNull())
    await setMotion(true)
    isShown.value = true
    await nextTick()
    const reducedDialog = screen.getByTestId('motion-dialog')
    expect(matchMedia('(prefers-reduced-motion: reduce)').matches).toBe(true)
    expect(styleOf(reducedDialog).transitionProperty).toBe('none')
    expect(styleOf(reducedDialog).transform).toBe('none')
    isShown.value = false
    await nextTick()
    expect(left?.duration).toBe('0s')
    await waitFor(() => expect(screen.queryByTestId('motion-dialog')).toBeNull())
  })

  it('renders control-state hover, active, loading, danger and focus without layout shifts', async () => {
    const control = document.createElement('button')
    control.textContent = 'Policy control'
    control.className = 'control-state'
    control.style.setProperty('--control-hover-transform', 'scale(1.02)')
    control.style.setProperty('--control-active-transform', 'scale(0.97)')
    control.style.setProperty('--control-danger-color', 'rgb(200, 20, 30)')
    control.style.setProperty('transition-property', 'color')
    document.body.append(control)
    const initialBorder = styleOf(control).borderWidth
    const initialWidth = control.offsetWidth
    await browserUser.hover(control)
    expect(styleOf(control).transform).toBe('matrix(1.02, 0, 0, 1.02, 0, 0)')
    await browserUser.keyboard('{Tab}')
    control.focus()
    expect(control.matches(':focus-visible')).toBe(true)
    expect(styleOf(control).outlineStyle).toBe('solid')
    expect(styleOf(control).outlineWidth).toBe('2px')
    expect(styleOf(control).outlineOffset).toBe('2px')
    expect(styleOf(control).borderWidth).toBe(initialBorder)
    expect(control.offsetWidth).toBe(initialWidth)
    await browserUser.keyboard('{Space>}')
    expect(styleOf(control).transform).toBe('matrix(0.97, 0, 0, 0.97, 0, 0)')
    await browserUser.keyboard('{/Space}')
    control.dataset.controlState = 'loading'
    expect(styleOf(control).cursor).toBe('wait')
    control.dataset.controlTone = 'danger'
    await waitFor(() => expect(styleOf(control).color).toBe('rgb(200, 20, 30)'))
    assertMotionTokens(control, false)
    await setMotion(true)
    expect(matchMedia('(prefers-reduced-motion: reduce)').matches).toBe(true)
    expect(styleOf(control).transform).toBe('none')
    // A real cascade regression must be observable by this computed-style oracle.
    const regression = document.createElement('style')
    regression.textContent = '.control-state:hover { transform: scale(2) !important; }'
    document.head.append(regression)
    try {
      expect(() => expect(styleOf(control).transform).toBe('none')).toThrow()
    } finally {
      regression.remove()
    }
    expect(styleOf(control).transform).toBe('none')
    control.remove()
  })

  it.each(['light', 'dark'] as const)(
    'renders real Input and Button focus state in %s mode',
    async (mode) => {
      document.documentElement.classList.toggle('dark', mode === 'dark')
      renderWithUser(
        defineComponent({
          setup: () => () =>
            h('div', [
              h(Input, { 'aria-label': 'Name', class: ibFormInputClass }),
              h(Button, { 'aria-label': 'Save', class: 'control-state' }, () => 'Save'),
            ]),
        }),
      )
      const input = screen.getByRole('textbox', { name: 'Name' })
      const button = screen.getByRole('button', { name: 'Save' })
      const unfocusedBorder = styleOf(input).borderColor
      const unfocusedWidth = styleOf(input).borderWidth
      await browserUser.keyboard('{Tab}')
      expect(document.activeElement).toBe(input)
      expect(input.matches(':focus-visible')).toBe(true)
      expect(styleOf(input).boxShadow).not.toBe('none')
      expect(styleOf(input).borderColor).toBe(unfocusedBorder)
      expect(styleOf(input).borderWidth).toBe(unfocusedWidth)
      expect(styleOf(input).transitionDuration).toBe('0.16s')
      expect(styleOf(input).transitionTimingFunction).toBe('cubic-bezier(0.2, 0.8, 0.2, 1)')
      assertMotionTokens(input, false)
      await browserUser.keyboard('{Tab}')
      expect(document.activeElement).toBe(button)
      expect(button.matches(':focus-visible')).toBe(true)
      expect(styleOf(button).boxShadow).not.toBe('none')
      await setMotion(true)
      expect(styleOf(input).transitionDuration).toBe('0s')
      assertMotionTokens(input, true)
      await page.screenshot({ path: `../../node_modules/.cache/p1-controls-${mode}.png` })
    },
  )

  it('lets the input surface own the focus border without primitive interference', async () => {
    renderWithUser(Input, {
      props: { class: 'focus-visible:border-destructive' },
      attrs: { 'aria-label': 'Surface input' },
    })
    const input = screen.getByRole('textbox', { name: 'Surface input' })
    const normalBorder = styleOf(input).borderColor
    await browserUser.keyboard('{Tab}')
    expect(input.matches(':focus-visible')).toBe(true)
    await waitFor(() => expect(styleOf(input).borderColor).not.toBe(normalBorder))
    const probe = document.createElement('div')
    probe.style.color = 'var(--destructive)'
    document.body.append(probe)
    await waitFor(() => expect(styleOf(input).borderColor).toBe(styleOf(probe).color))
    probe.remove()
  })

  it('keeps real Button disabled and loading semantics stable', async () => {
    const action = vi.fn()
    const { rerender, unmount } = renderWithUser(Button, {
      props: { isLoading: true },
      attrs: { onClick: action },
      slots: { default: 'Action' },
    })
    const loadingAction = screen.getByRole('button', { name: 'Action' })
    loadingAction.click()
    expect(action).not.toHaveBeenCalled()
    await rerender({ isLoading: false })
    loadingAction.click()
    expect(action).toHaveBeenCalledOnce()
    action.mockClear()
    renderWithUser(
      defineComponent({
        setup: () => () =>
          h('div', [
            h(Button, { isDisabled: true, 'aria-label': 'Disabled' }, () => 'Disabled'),
            h(
              Button,
              { isLoading: true, 'aria-label': 'Loading', onClick: action },
              () => 'Loading',
            ),
          ]),
      }),
    )
    const disabled = screen.getByRole('button', { name: 'Disabled' })
    const loading = screen.getByRole('button', { name: 'Loading' })
    expect(disabled).toBeDisabled()
    expect(loading).toBeDisabled()
    expect(loading).toHaveAttribute('aria-busy', 'true')
    loading.click()
    disabled.click()
    expect(action).not.toHaveBeenCalled()
    expect(Number(styleOf(loading).opacity)).toBeLessThan(1)
    unmount()
    await browserUser.keyboard('{Tab}{Enter}')
    expect(document.activeElement).not.toBe(disabled)
    expect(document.activeElement).not.toBe(loading)
    expect(action).not.toHaveBeenCalled()
  })

  it('forwards ActionButton aria-label and title in the real DOM', () => {
    renderWithUser(ActionButton, {
      props: { ariaLabel: 'Export', title: 'Export image', label: '' },
    })
    const button = screen.getByRole('button', { name: 'Export' })
    expect(button).toHaveAttribute('aria-label', 'Export')
    expect(button).toHaveAttribute('title', 'Export image')
  })

  it.each(['light', 'dark'] as const)(
    'renders the real appearance menu with visible keyboard focus in %s mode',
    async (mode) => {
      useTheme().setTheme(mode)
      renderWithUser(UserAccountMenu, { props: { items: [] } })
      await browserUser.keyboard('{Tab}{Enter}')
      const appearance = await screen.findByRole('menuitem', { name: 'Appearance' })
      await browserUser.keyboard('{ArrowRight}')
      await waitFor(() => expect(document.activeElement).toBe(appearance))
      await browserUser.keyboard('{Enter}')
      const dark = await screen.findByRole('menuitem', { name: 'Dark' })
      await browserUser.keyboard('{ArrowDown}')
      await waitFor(() => expect(document.activeElement).toBe(dark))
      expect(dark.matches(':focus-visible')).toBe(true)
      expect(styleOf(dark).transitionDuration).toBe('0.16s')
      expect(styleOf(dark).transitionTimingFunction).toBe('cubic-bezier(0.2, 0.8, 0.2, 1)')
      expect(styleOf(dark).outlineWidth).toBe('2px')
      expect(styleOf(dark).outlineOffset).toBe('2px')
      assertMotionTokens(dark, false)
      await setMotion(true)
      expect(styleOf(dark).transitionDuration).toBe('0s')
      assertMotionTokens(dark, true)
      expect(styleOf(dark).transform).toBe('none')
      await page.screenshot({ path: `../../node_modules/.cache/p1-menu-${mode}.png` })
      await browserUser.keyboard('{Enter}')
      await waitFor(() => expect(document.documentElement.classList.contains('dark')).toBe(true))
    },
  )

  it.each([
    ['light', false],
    ['dark', false],
    ['light', true],
    ['dark', true],
  ] as const)(
    'renders real UserConfirmDialog focus and loading in %s (reduced=%s)',
    async (mode, isReduced) => {
      useTheme().setTheme(mode)
      await setMotion(isReduced)
      const confirmAction = vi.fn()
      const { rerender, emitted } = renderWithUser(UserConfirmDialog, {
        props: {
          isVisible: true,
          title: 'Delete item',
          message: 'Cannot undo',
          isLoading: false,
          onConfirm: confirmAction,
        },
      })
      const dialog = await screen.findByRole('dialog', { name: 'Delete item' })
      const cancel = screen.getByRole('button', { name: 'Cancel' })
      const confirm = screen.getByRole('button', { name: 'Confirm' })
      await browserUser.keyboard('{Tab}')
      expect(document.activeElement).toBe(confirm)
      expect(confirm.matches(':focus-visible')).toBe(true)
      expect(styleOf(confirm).boxShadow).not.toBe('none')
      expect(styleOf(dialog).backgroundColor).not.toBe('rgba(0, 0, 0, 0)')
      expect(styleOf(dialog).animationName).toBe(isReduced ? 'none' : 'overlay-enter')
      await rerender({ isLoading: true })
      expect(dialog).toHaveAttribute('aria-busy', 'true')
      expect(cancel).toBeDisabled()
      expect(confirm).toBeDisabled()
      await browserUser.keyboard('{Escape}')
      expect(emitted()['update:visible']).toBeUndefined()
      expect(confirmAction).not.toHaveBeenCalled()
      await page.screenshot({
        path: `../../node_modules/.cache/p1-user-confirm-${mode}-${isReduced}.png`,
      })
    },
  )

  it.each([
    ['light', false],
    ['dark', false],
    ['light', true],
    ['dark', true],
  ] as const)(
    'renders real AdminConfirmDialog focus and loading in %s (reduced=%s)',
    async (mode, isReduced) => {
      useTheme().setTheme(mode)
      await setMotion(isReduced)
      let settle!: () => void
      const action = vi.fn(
        () =>
          new Promise<void>((resolve) => {
            settle = resolve
          }),
      )
      const pending = useAdminConfirm().confirm({
        title: 'Delete user',
        message: 'Audit action',
        confirmLabel: 'Delete',
        intent: 'danger',
        onConfirm: action,
      })
      renderWithUser(AdminConfirmDialog)
      const dialog = await screen.findByRole('alertdialog', { name: 'Delete user' })
      const cancel = screen.getByRole('button', { name: '取消' })
      const confirm = screen.getByRole('button', { name: 'Delete' })
      expect(screen.queryByRole('textbox')).toBeNull()
      expect(confirm).toBeEnabled()
      cancel.focus()
      await browserUser.keyboard('{Tab}')
      expect(document.activeElement).toBe(confirm)
      expect(confirm.matches(':focus-visible')).toBe(true)
      expect(styleOf(confirm).outlineWidth).not.toBe('0px')
      expect(styleOf(dialog).animationName).toBe(isReduced ? 'none' : 'overlay-enter')
      await browserUser.keyboard('{Enter}')
      await waitFor(() => expect(confirm).toBeDisabled())
      expect(screen.getByRole('button', { name: '取消' })).toBeDisabled()
      expect(action).toHaveBeenCalledOnce()
      await page.screenshot({
        path: `../../node_modules/.cache/p1-admin-confirm-${mode}-${isReduced}.png`,
      })
      settle()
      await expect(pending).resolves.toBe(true)
    },
  )
})
