import { describe, expect, it, vi } from 'vitest'
import { h, nextTick } from 'vue'
import { renderWithUser, screen, within } from '../../../../../test/render'
import AdminDialogShell from './AdminDialogShell.vue'

function renderShell(
  props: {
    visible?: boolean
    header?: string | undefined
    ariaLabel?: string
    size?: 'compact' | 'form' | 'wide'
    contentLayout?: 'form' | 'scrollable'
    busy?: boolean
  } = {},
  slots: { header?: string } = {},
) {
  const onVisibleUpdate = vi.fn()
  const onClose = vi.fn()
  return {
    onVisibleUpdate,
    onClose,
    ...renderWithUser(AdminDialogShell, {
      props: {
        visible: true,
        header: '编辑配置',
        description: '修改配置项后保存生效。',
        ...props,
        'onUpdate:visible': onVisibleUpdate,
        onClose,
      },
      slots: {
        ...slots,
        default:
          '<button type="button">第一个操作</button><button type="button">第二个操作</button>',
        footer: '<button type="button">取消</button><button type="button">保存</button>',
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
        description: '修改配置项后保存生效。',
        onReset,
      },
    })

    await result.rerender({
      visible: true,
      header: '编辑配置',
      description: '修改配置项后保存生效。',
      onReset,
    })
    expect(onReset).toHaveBeenCalledTimes(1)

    await result.rerender({
      visible: false,
      header: '编辑配置',
      description: '修改配置项后保存生效。',
      onReset,
    })
    await result.rerender({
      visible: true,
      header: '编辑配置',
      description: '修改配置项后保存生效。',
      onReset,
    })
    expect(onReset).toHaveBeenCalledTimes(2)
  })

  it('gives custom headers a stable accessible name', async () => {
    renderShell(
      { header: undefined, ariaLabel: '编辑节点提示词' },
      { header: '<h2>节点提示词</h2>' },
    )

    expect(await screen.findByRole('dialog', { name: '编辑节点提示词' })).toBeInTheDocument()
  })

  it('uses the shared Xmark close icon', async () => {
    renderShell()

    const dialog = await screen.findByRole('dialog', { name: '编辑配置' })
    expect(dialog).toHaveClass('bg-popover')
    for (const region of ['header', 'content', 'footer']) {
      expect(dialog.querySelector(`[data-admin-dialog-region="${region}"]`)).toHaveClass(
        'bg-popover',
      )
    }
    const scrollViewport = dialog.querySelector('[data-admin-dialog-region="content"] > div')
    expect(scrollViewport).toHaveClass('-mx-3', 'px-3', 'overflow-y-auto')
    const icon = within(dialog)
      .getByRole('button', { name: /close|关闭/i })
      .querySelector('[data-icon="xmark"]')
    expect(icon).toBeInTheDocument()
    expect(icon).toHaveClass('!h-5', '!w-5')
  })

  it('traps focus inside the dialog in both tab directions', async () => {
    const { user } = renderShell()
    const dialog = await screen.findByRole('dialog', { name: '编辑配置' })
    const buttons = within(dialog).getAllByRole('button')

    buttons.at(-1)!.focus()
    await user.tab()
    expect(buttons[0]).toHaveFocus()

    buttons[0]!.focus()
    await user.tab({ shift: true })
    expect(buttons.at(-1)).toHaveFocus()
  })

  it('closes through mask click like AuthModalHost and BoardsPage', async () => {
    const { user, onVisibleUpdate, onClose } = renderShell()
    const dialog = await screen.findByRole('dialog', { name: '编辑配置' })
    const mask = dialog.parentElement!.querySelector('[data-admin-dialog-mask]')!

    expect(dialog).toHaveAttribute('aria-modal', 'true')
    await user.click(mask)
    expect(onVisibleUpdate).toHaveBeenCalledWith(false)
    expect(onClose).toHaveBeenCalledTimes(1)
  })

  it('closes through Escape', async () => {
    const { user, onVisibleUpdate } = renderShell()
    const dialog = await screen.findByRole('dialog', { name: '编辑配置' })

    dialog.focus()
    await user.keyboard('{Escape}')
    expect(onVisibleUpdate).toHaveBeenCalledWith(false)
  })

  it('routes the header close control through the lifecycle', async () => {
    const { user, onVisibleUpdate, onClose } = renderShell()
    const dialog = await screen.findByRole('dialog', { name: '编辑配置' })

    expect(within(dialog).getByRole('button', { name: /close|关闭/i })).toHaveClass(
      '!bg-transparent',
      '!shadow-none',
    )
    await user.click(within(dialog).getByRole('button', { name: /close|关闭/i }))
    expect(onVisibleUpdate).toHaveBeenCalledWith(false)
    expect(onClose).toHaveBeenCalledTimes(1)
  })

  it('blocks every internal close path while busy', async () => {
    const onVisibleUpdate = vi.fn()
    const { user } = renderWithUser(AdminDialogShell, {
      props: {
        visible: true,
        header: '编辑配置',
        description: '修改配置项后保存生效。',
        busy: true,
        'onUpdate:visible': onVisibleUpdate,
      },
      slots: {
        footer: ({ requestClose }: { requestClose: () => void }) =>
          h('button', { type: 'button', onClick: requestClose }, '取消'),
      },
    })
    const dialog = await screen.findByRole('dialog', { name: '编辑配置' })
    const mask = dialog.parentElement!.querySelector('[data-admin-dialog-mask]')!
    expect(dialog).toHaveAttribute('aria-busy', 'true')
    expect(within(dialog).queryByRole('button', { name: /close|关闭/i })).not.toBeInTheDocument()

    await user.click(mask)
    await user.click(within(dialog).getByRole('button', { name: '取消' }))
    dialog.focus()
    await user.keyboard('{Escape}')
    expect(onVisibleUpdate).not.toHaveBeenCalled()
  })

  it('restores focus only after the exit animation finishes', async () => {
    const readStyle = getComputedStyle
    const styleSpy = vi.spyOn(globalThis, 'getComputedStyle').mockImplementation((element) => {
      const style = readStyle(element)
      if (!element.classList.contains('overlay-dialog-motion')) return style
      return new Proxy(style, {
        get(target, property) {
          if (property === 'animationName') {
            return element.getAttribute('data-state') === 'open' ? 'overlay-enter' : 'overlay-exit'
          }
          const value = Reflect.get(target, property, target)
          return typeof value === 'function' ? value.bind(target) : value
        },
      })
    })
    const trigger = document.createElement('button')
    trigger.textContent = '打开配置'
    document.body.append(trigger)
    trigger.focus()
    const result = renderShell()

    try {
      const dialog = await screen.findByRole('dialog', { name: '编辑配置' })
      dialog.focus()
      dialog.dispatchEvent(
        Object.assign(new Event('animationstart'), { animationName: 'overlay-enter' }),
      )
      await result.user.keyboard('{Escape}')
      await result.rerender({ visible: false, header: '编辑配置' })
      await nextTick()

      expect(dialog).toBeInTheDocument()
      expect(dialog).toHaveAttribute('data-state', 'closed')
      expect(trigger).not.toHaveFocus()
      dialog.dispatchEvent(
        Object.assign(new Event('animationcancel'), { animationName: 'overlay-enter' }),
      )
      expect(dialog).toBeInTheDocument()
      dialog.dispatchEvent(
        Object.assign(new Event('animationend'), { animationName: 'overlay-exit' }),
      )
      await vi.waitFor(() => expect(dialog).not.toBeInTheDocument())
      await vi.waitFor(() => expect(trigger).toHaveFocus())
    } finally {
      styleSpy.mockRestore()
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
      '-mx-3',
      'overflow-y-auto',
      'overscroll-contain',
      'px-3',
      'max-h-[min(68dvh,40rem)]',
    )
  })

  it('aligns header, content, and footer to one horizontal rail', async () => {
    renderShell()

    const dialog = await screen.findByRole('dialog', { name: '编辑配置' })
    for (const region of ['header', 'content', 'footer']) {
      expect(dialog.querySelector(`[data-admin-dialog-region="${region}"]`)).toHaveClass('!px-6')
    }
  })

  it.each([
    ['compact', 'w-[min(30rem,calc(100vw-1.5rem))]'],
    ['form', 'w-[min(35rem,calc(100vw-1.5rem))]'],
    ['wide', 'w-[min(61.25rem,calc(100vw-1.5rem))]'],
  ] as const)('owns the %s desktop and mobile width contract', async (size, expectedClass) => {
    renderShell({ size })

    const dialog = await screen.findByRole('dialog', { name: '编辑配置' })
    expect(dialog).toHaveClass(expectedClass)
    expect(dialog).toHaveAttribute('data-size', size)
  })

  it('renders footer actions through the shared footer layout', async () => {
    renderShell()

    const footer = (await screen.findByRole('button', { name: '保存' })).closest(
      '[data-admin-dialog-footer]',
    )
    expect(footer).toHaveClass('flex', 'flex-wrap', 'justify-end', 'gap-2')
  })
})
