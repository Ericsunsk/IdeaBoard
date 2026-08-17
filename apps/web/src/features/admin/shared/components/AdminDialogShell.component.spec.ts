import PrimeVue from 'primevue/config'
import { describe, expect, it, vi } from 'vitest'
import { h } from 'vue'
import { primeVueTestOptions } from '../../../../test/primeVue'
import { renderWithUser, screen, within } from '../../../../test/render'
import AdminDialogShell from './AdminDialogShell.vue'

function renderShell(
  props: Partial<{
    visible: boolean
    header: string
    ariaLabel: string
    size: 'compact' | 'form' | 'wide'
    contentLayout: 'form' | 'scrollable'
    busy: boolean
  }> = {},
  slots: { header?: string } = {},
) {
  const onVisibleUpdate = vi.fn()
  return {
    onVisibleUpdate,
    ...renderWithUser(AdminDialogShell, {
      props: {
        visible: true,
        header: '编辑配置',
        ...props,
        'onUpdate:visible': onVisibleUpdate,
      },
      slots: {
        ...slots,
        default:
          '<button type="button">第一个操作</button><button type="button">第二个操作</button>',
        footer: '<button type="button">取消</button><button type="button">保存</button>',
      },
      global: {
        plugins: [[PrimeVue, primeVueTestOptions]],
        stubs: { transition: false },
      },
    }),
  }
}

describe('AdminDialogShell', () => {
  it('resets transient state on every open', async () => {
    const onReset = vi.fn()
    const result = renderWithUser(AdminDialogShell, {
      props: {
        visible: false,
        header: '编辑配置',
        onReset,
      },
      global: {
        plugins: [[PrimeVue, primeVueTestOptions]],
        stubs: { transition: false },
      },
    })

    await result.rerender({ visible: true, header: '编辑配置', onReset })
    expect(onReset).toHaveBeenCalledTimes(1)

    await result.rerender({ visible: false, header: '编辑配置', onReset })
    await result.rerender({ visible: true, header: '编辑配置', onReset })
    expect(onReset).toHaveBeenCalledTimes(2)
  })

  it('gives custom headers a stable accessible name', async () => {
    renderShell(
      { header: undefined, ariaLabel: '编辑节点提示词' },
      { header: '<h2>节点提示词</h2>' },
    )

    expect(await screen.findByRole('dialog', { name: '编辑节点提示词' })).toBeInTheDocument()
  })

  it('uses the real PrimeVue focus trap to wrap both tab directions', async () => {
    renderShell()
    const dialog = await screen.findByRole('dialog', { name: '编辑配置' })
    const buttons = within(dialog).getAllByRole('button')
    const lastButton = buttons.at(-1)!
    const firstFocusGuard = dialog.querySelector<HTMLElement>(
      '[data-pc-section="firstfocusableelement"]',
    )!
    const lastFocusGuard = dialog.querySelector<HTMLElement>(
      '[data-pc-section="lastfocusableelement"]',
    )!

    lastButton.focus()
    lastFocusGuard.focus()
    const firstFocusable = document.activeElement
    expect(firstFocusable).toBeInstanceOf(HTMLButtonElement)
    expect(dialog).toContainElement(firstFocusable as HTMLElement)

    firstFocusGuard.focus()
    expect(lastButton).toHaveFocus()
  })

  it('closes through Escape while preventing mask dismissal', async () => {
    const { user, onVisibleUpdate } = renderShell()
    const dialog = await screen.findByRole('dialog', { name: '编辑配置' })
    const mask = dialog.parentElement!

    expect(dialog).toHaveAttribute('aria-modal', 'true')
    await user.click(mask)
    expect(onVisibleUpdate).not.toHaveBeenCalled()

    dialog.focus()
    await user.keyboard('{Escape}')
    expect(onVisibleUpdate).toHaveBeenCalledWith(false)
  })

  it('routes the header close control through the lifecycle', async () => {
    const { user, onVisibleUpdate } = renderShell()
    const dialog = await screen.findByRole('dialog', { name: '编辑配置' })

    await user.click(within(dialog).getByRole('button', { name: /close|关闭/i }))
    expect(onVisibleUpdate).toHaveBeenCalledWith(false)
  })

  it('blocks every internal close path while busy', async () => {
    const onVisibleUpdate = vi.fn()
    const { user } = renderWithUser(AdminDialogShell, {
      props: {
        visible: true,
        header: '编辑配置',
        busy: true,
        'onUpdate:visible': onVisibleUpdate,
      },
      slots: {
        footer: ({ requestClose }: { requestClose: () => void }) =>
          h('button', { type: 'button', onClick: requestClose }, '取消'),
      },
      global: {
        plugins: [[PrimeVue, primeVueTestOptions]],
        stubs: { transition: false },
      },
    })
    const dialog = await screen.findByRole('dialog', { name: '编辑配置' })
    const mask = dialog.parentElement!

    expect(dialog).toHaveAttribute('aria-busy', 'true')
    expect(within(dialog).queryByRole('button', { name: /close|关闭/i })).not.toBeInTheDocument()

    await user.click(mask)
    await user.click(within(dialog).getByRole('button', { name: '取消' }))
    dialog.focus()
    await user.keyboard('{Escape}')
    expect(onVisibleUpdate).not.toHaveBeenCalled()
  })

  it('restores focus to the trigger after the dialog has closed', async () => {
    const trigger = document.createElement('button')
    trigger.textContent = '打开配置'
    document.body.append(trigger)
    trigger.focus()
    const result = renderShell()

    try {
      const dialog = await screen.findByRole('dialog', { name: '编辑配置' })
      dialog.focus()
      await result.user.keyboard('{Escape}')
      await result.rerender({ visible: false, header: '编辑配置' })

      await vi.waitFor(() => expect(trigger).toHaveFocus())
    } finally {
      trigger.remove()
    }
  })

  it('does not restore focus to a stale trigger when the dialog reopens during leave', async () => {
    const trigger = document.createElement('button')
    trigger.textContent = '打开配置'
    document.body.append(trigger)
    trigger.focus()
    const triggerFocus = vi.spyOn(trigger, 'focus')
    const result = renderShell()

    try {
      await screen.findByRole('dialog', { name: '编辑配置' })
      await result.rerender({ visible: false, header: '编辑配置' })
      await result.rerender({ visible: true, header: '编辑配置' })

      expect(triggerFocus).not.toHaveBeenCalled()
      expect(await screen.findByRole('dialog', { name: '编辑配置' })).toBeInTheDocument()
    } finally {
      trigger.remove()
    }
  })

  it('owns the form scroll viewport', async () => {
    renderShell()

    const content = (await screen.findByRole('button', { name: '第一个操作' })).parentElement
    expect(content).toHaveClass(
      'app-scrollbar',
      '-mr-3',
      'overflow-y-auto',
      'overscroll-contain',
      'pr-3',
      'max-h-[min(68dvh,40rem)]',
    )
  })

  it('aligns header, content, and footer to one horizontal rail', async () => {
    renderShell()

    const dialog = await screen.findByRole('dialog', { name: '编辑配置' })
    expect(dialog.querySelector('[data-pc-section="header"]')).toHaveClass('!px-6')
    expect(dialog.querySelector('[data-pc-section="content"]')).toHaveClass('!px-6')
    expect(dialog.querySelector('[data-pc-section="footer"]')).toHaveClass('!px-6')
  })

  it.each([
    ['compact', 'w-[min(30rem,calc(100vw-1.5rem))]'],
    ['form', 'w-[min(35rem,calc(100vw-1.5rem))]'],
    ['wide', 'w-[min(61.25rem,calc(100vw-1.5rem))]'],
  ] as const)('owns the %s desktop and mobile width contract', async (size, expectedClass) => {
    renderShell({ size })

    expect(await screen.findByRole('dialog', { name: '编辑配置' })).toHaveClass(expectedClass)
  })

  it('renders footer actions through the shared footer layout', async () => {
    renderShell()

    const footer = (await screen.findByRole('button', { name: '保存' })).closest(
      '[data-admin-dialog-footer]',
    )
    expect(footer).toHaveClass('flex', 'flex-wrap', 'justify-end', 'gap-2')
  })
})
