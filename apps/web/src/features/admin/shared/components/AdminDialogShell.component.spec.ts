import PrimeVue from 'primevue/config'
import { describe, expect, it, vi } from 'vitest'
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

  it('closes on Escape while preventing accidental mask dismissal', async () => {
    const { user, onVisibleUpdate } = renderShell()
    const dialog = await screen.findByRole('dialog', { name: '编辑配置' })

    expect(dialog).toHaveAttribute('aria-modal', 'true')
    dialog.focus()
    await user.keyboard('{Escape}')
    expect(onVisibleUpdate).toHaveBeenCalledWith(false)
  })

  it('blocks every internal close path while busy', async () => {
    const { user, onVisibleUpdate } = renderShell({ busy: true })
    const dialog = await screen.findByRole('dialog', { name: '编辑配置' })

    expect(dialog).toHaveAttribute('aria-busy', 'true')
    expect(within(dialog).queryByRole('button', { name: /close|关闭/i })).not.toBeInTheDocument()

    dialog.focus()
    await user.keyboard('{Escape}')
    expect(onVisibleUpdate).not.toHaveBeenCalled()
  })

  it('owns the form scroll viewport', async () => {
    renderShell()

    const content = (await screen.findByRole('button', { name: '第一个操作' })).parentElement
    expect(content).toHaveClass(
      'app-scrollbar',
      'overflow-y-auto',
      'overscroll-contain',
      'max-h-[min(68dvh,40rem)]',
    )
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
