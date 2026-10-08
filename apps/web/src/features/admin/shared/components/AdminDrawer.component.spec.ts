import { describe, expect, it, vi } from 'vitest'
import { renderWithUser, screen, within } from '../../../../test/render'
import AdminDrawer from './AdminDrawer.vue'

function renderDrawer(
  props: {
    visible?: boolean
    title?: string
    description?: string
    closeLabel?: string
    busy?: boolean
  } = {},
) {
  const onVisibleUpdate = vi.fn()
  const onClose = vi.fn()
  return {
    onVisibleUpdate,
    onClose,
    ...renderWithUser(AdminDrawer, {
      props: {
        visible: true,
        title: '诊断详情',
        description: '某条发现项的诊断依据。',
        closeLabel: '关闭',
        ...props,
        'onUpdate:visible': onVisibleUpdate,
        onClose,
      },
      slots: {
        default: '<p>抽屉正文</p>',
      },
    }),
  }
}

describe('AdminDrawer', () => {
  it('renders as a right-anchored dialog named by its title', async () => {
    renderDrawer()

    const dialog = await screen.findByRole('dialog', { name: '诊断详情' })
    expect(dialog).toHaveAttribute('data-admin-drawer')
    expect(dialog).toHaveClass('inset-y-0', 'right-0')
    expect(within(dialog).getByText('抽屉正文')).toBeInTheDocument()
    expect(dialog.querySelector('[data-admin-drawer-region="header"]')).toBeInTheDocument()
    expect(dialog.querySelector('[data-admin-drawer-region="content"]')).toBeInTheDocument()
  })

  it('closes through the header close control', async () => {
    const { user, onVisibleUpdate, onClose } = renderDrawer()
    const dialog = await screen.findByRole('dialog', { name: '诊断详情' })

    await user.click(within(dialog).getByRole('button', { name: '关闭' }))
    expect(onVisibleUpdate).toHaveBeenCalledWith(false)
    expect(onClose).toHaveBeenCalledTimes(1)
  })

  it('closes through mask click', async () => {
    const { user, onVisibleUpdate, onClose } = renderDrawer()
    const dialog = await screen.findByRole('dialog', { name: '诊断详情' })
    const mask = dialog.parentElement!.querySelector('[data-admin-drawer-mask]')!

    await user.click(mask)
    expect(onVisibleUpdate).toHaveBeenCalledWith(false)
    expect(onClose).toHaveBeenCalledTimes(1)
  })

  it('closes through Escape', async () => {
    const { user, onVisibleUpdate } = renderDrawer()
    const dialog = await screen.findByRole('dialog', { name: '诊断详情' })

    dialog.focus()
    await user.keyboard('{Escape}')
    expect(onVisibleUpdate).toHaveBeenCalledWith(false)
  })

  it('blocks every close path while busy', async () => {
    const onVisibleUpdate = vi.fn()
    const { user } = renderWithUser(AdminDrawer, {
      props: {
        visible: true,
        title: '诊断详情',
        description: '某条发现项的诊断依据。',
        busy: true,
        'onUpdate:visible': onVisibleUpdate,
      },
      slots: { default: '<p>抽屉正文</p>' },
    })
    const dialog = await screen.findByRole('dialog', { name: '诊断详情' })
    const mask = dialog.parentElement!.querySelector('[data-admin-drawer-mask]')!

    expect(dialog).toHaveAttribute('aria-busy', 'true')
    expect(within(dialog).queryByRole('button', { name: '关闭' })).not.toBeInTheDocument()

    await user.click(mask)
    dialog.focus()
    await user.keyboard('{Escape}')
    expect(onVisibleUpdate).not.toHaveBeenCalled()
  })

  it('renders an optional footer region', async () => {
    renderWithUser(AdminDrawer, {
      props: { visible: true, title: '诊断详情', description: '某条发现项的诊断依据。' },
      slots: {
        default: '<p>抽屉正文</p>',
        footer: '<button type="button">知道了</button>',
      },
    })

    const dialog = await screen.findByRole('dialog', { name: '诊断详情' })
    expect(dialog.querySelector('[data-admin-drawer-region="footer"]')).toBeInTheDocument()
  })
})
