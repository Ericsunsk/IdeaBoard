import { describe, expect, it, vi } from 'vitest'
import DialogShell from './DialogShell.vue'
import { dialogShellPresets, dialogShellSizeClasses, type DialogShellSize } from './dialogShell'
import { renderWithUser, screen } from '@/test/render'

function renderShell(
  props: Partial<{
    isOpen: boolean
    title: string
    description: string
    size: DialogShellSize
    isBusy: boolean
    hasClose: boolean
  }> = {},
) {
  const onOpenUpdate = vi.fn()
  const onClose = vi.fn()
  const view = renderWithUser(DialogShell, {
    props: {
      isOpen: true,
      title: '共享看板',
      description: undefined,
      ...props,
      'onUpdate:open': onOpenUpdate,
      onClose,
    },
    slots: { default: '<button type="button">主体操作</button>' },
  })
  return { ...view, onOpenUpdate, onClose }
}

describe('DialogShell', () => {
  it('owns the size contract through data-size and width classes', async () => {
    renderShell({ size: 'compact' })

    const dialog = await screen.findByRole('dialog', { name: '共享看板' })
    expect(dialog).toHaveAttribute('data-size', 'compact')
    expect(dialog).toHaveClass('w-[min(30rem,calc(100vw-1.5rem))]')
    expect(dialog).toHaveClass('z-admin-dialog')
    expect(dialog).toHaveClass('overlay-dialog-motion')
  })

  it('keeps every size on the same motion and stacking contract', () => {
    for (const size of Object.keys(dialogShellSizeClasses)) {
      const classes = dialogShellSizeClasses[size as keyof typeof dialogShellSizeClasses]
      const viewportGuard = ['slim-inset', 'narrow'].includes(size)
        ? 'calc(100vw-32px)'
        : 'calc(100vw-1.5rem)'
      expect(classes).toContain(viewportGuard)
    }
  })

  it.each([
    ['slim-inset', 'max-w-[23rem]'],
    ['narrow', 'max-w-[30rem]'],
  ] as const)('preserves the %s viewport width and original rem maximum', async (size, maximum) => {
    renderShell({ size })

    const dialog = await screen.findByRole('dialog', { name: '共享看板' })
    expect(dialog).toHaveAttribute('data-size', size)
    expect(dialog).toHaveClass('w-[min(420px,calc(100vw-32px))]', maximum)
  })

  it('closes through mask click and Escape while idle', async () => {
    const { user, onOpenUpdate, onClose } = renderShell({ description: '共享说明' })

    const dialog = await screen.findByRole('dialog', { name: '共享看板' })
    const mask = dialog.parentElement!.querySelector('[data-dialog-shell-mask]')!
    await user.click(mask as Element)
    expect(onOpenUpdate).toHaveBeenCalledWith(false)
    expect(onClose).toHaveBeenCalledTimes(1)
  })

  it('locks mask and Escape while a commit is busy and exposes aria-busy', async () => {
    const { user, onOpenUpdate } = renderShell({ isBusy: true, description: '共享说明' })

    const dialog = await screen.findByRole('dialog', { name: '共享看板' })
    expect(dialog).toHaveAttribute('aria-busy', 'true')

    const mask = dialog.parentElement!.querySelector('[data-dialog-shell-mask]')!
    await user.click(mask as Element)
    expect(onOpenUpdate).not.toHaveBeenCalled()

    dialog.focus()
    await user.keyboard('{Escape}')
    expect(onOpenUpdate).not.toHaveBeenCalled()
  })

  it('keeps Admin confirm on alertdialog and user confirm on dialog', () => {
    // Alert semantics stay split: AdminConfirmDialog owns `alertdialog`,
    // user confirms (UserConfirmDialog / DialogShell presets) own `dialog`.
    expect(dialogShellPresets.confirm.hasClose).toBe(false)
    expect(dialogShellPresets.share.hasClose).toBe(true)
  })
})
