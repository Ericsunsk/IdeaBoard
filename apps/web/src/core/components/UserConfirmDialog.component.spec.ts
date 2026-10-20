import { defineComponent } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import UserConfirmDialog from './UserConfirmDialog.vue'
import { renderWithUser, screen } from '../../test/render'

const DialogRootStub = defineComponent({
  props: {
    open: { type: Boolean, required: true },
  },
  emits: ['update:open'],
  template: `
    <div v-if="open" @keydown.escape="$emit('update:open', false)">
      <slot />
    </div>
  `,
})

const PassthroughStub = defineComponent({
  template: '<slot />',
})

const DialogOverlayStub = defineComponent({
  inheritAttrs: false,
  emits: ['click'],
  template:
    '<div data-testid="user-confirm-mask" v-bind="$attrs" @click="$emit(\'click\', $event)"><slot /></div>',
})

const DialogContentStub = defineComponent({
  inheritAttrs: false,
  template: `
    <section role="dialog" tabindex="-1" v-bind="$attrs" @click.stop>
      <slot />
    </section>
  `,
})

function renderConfirmDialog(isLoading = false, extraProps: Record<string, unknown> = {}) {
  const onVisibleUpdate = vi.fn()
  const onConfirm = vi.fn()
  const view = renderWithUser(UserConfirmDialog, {
    props: {
      isVisible: true,
      title: '删除确认',
      message: '确定删除吗？',
      confirmLabel: '删除',
      intent: 'danger',
      isLoading,
      'onUpdate:isVisible': onVisibleUpdate,
      onConfirm,
      ...extraProps,
    },
    global: {
      stubs: {
        DialogRoot: DialogRootStub,
        DialogPortal: PassthroughStub,
        DialogOverlay: DialogOverlayStub,
        DialogContent: DialogContentStub,
        DialogDescription: PassthroughStub,
        DialogTitle: PassthroughStub,
      },
    },
  })
  return { ...view, onVisibleUpdate, onConfirm }
}

describe('UserConfirmDialog', () => {
  it('dismisses from the mask through the shared lifecycle while idle', async () => {
    const { user, onVisibleUpdate } = renderConfirmDialog()

    await user.click(screen.getByTestId('user-confirm-mask'))
    expect(onVisibleUpdate).toHaveBeenCalledWith(false)
  })

  it('dismisses from Escape through the shared lifecycle while idle', async () => {
    const { user, onVisibleUpdate } = renderConfirmDialog()

    screen.getByRole('dialog', { name: '删除确认' }).focus()
    await user.keyboard('{Escape}')
    expect(onVisibleUpdate).toHaveBeenCalledWith(false)
  })

  it('emits confirm and cancel through its own controls while idle', async () => {
    const { user, onVisibleUpdate, onConfirm } = renderConfirmDialog()

    await user.click(screen.getByRole('button', { name: '删除' }))
    expect(onConfirm).toHaveBeenCalledTimes(1)

    await user.click(screen.getByRole('button', { name: /cancel|取消/i }))
    expect(onVisibleUpdate).toHaveBeenCalledWith(false)
  })

  it('locks mask, Escape, and external close while the irreversible commit is busy', async () => {
    const { user, onVisibleUpdate } = renderConfirmDialog(true)
    const dialog = screen.getByRole('dialog', { name: '删除确认' })

    expect(dialog).toHaveAttribute('aria-busy', 'true')

    await user.click(screen.getByTestId('user-confirm-mask'))
    expect(onVisibleUpdate).not.toHaveBeenCalled()

    dialog.focus()
    await user.keyboard('{Escape}')
    expect(onVisibleUpdate).not.toHaveBeenCalled()
  })

  it('disables confirm until the typed text matches the expected name', async () => {
    const { user, onConfirm } = renderConfirmDialog(false, {
      expectedConfirmText: 'Team',
      confirmInputLabel: 'Type "Team" to confirm',
      confirmInputPlaceholder: 'Workspace name',
    })

    const confirmButton = screen.getByRole('button', { name: '删除' })
    const nameInput = screen.getByLabelText('Type "Team" to confirm')
    expect(nameInput).toHaveAttribute('placeholder', 'Workspace name')
    expect(confirmButton).toBeDisabled()

    await user.type(nameInput, 'Tea')
    expect(confirmButton).toBeDisabled()

    await user.clear(nameInput)
    await user.type(nameInput, 'Team')
    expect(confirmButton).toBeEnabled()

    await user.click(confirmButton)
    expect(onConfirm).toHaveBeenCalledTimes(1)
  })

  it('keeps the typed confirm locked while busy and confirms on Enter when matched', async () => {
    const { user, onConfirm } = renderConfirmDialog(false, {
      expectedConfirmText: 'Team',
      confirmInputLabel: 'Type "Team" to confirm',
    })

    const nameInput = screen.getByLabelText('Type "Team" to confirm')
    await user.type(nameInput, 'Team')
    await user.keyboard('{Enter}')
    expect(onConfirm).toHaveBeenCalledTimes(1)
  })

  it('disables the typed input and confirm while the commit is busy', async () => {
    const { user, onConfirm } = renderConfirmDialog(true, {
      expectedConfirmText: 'Team',
      confirmInputLabel: 'Type "Team" to confirm',
    })

    const nameInput = screen.getByLabelText('Type "Team" to confirm')
    expect(nameInput).toBeDisabled()
    expect(screen.getByRole('button', { name: '删除' })).toBeDisabled()

    await user.keyboard('{Enter}')
    expect(onConfirm).not.toHaveBeenCalled()
  })

  it('copies the expected name when its chip is clicked', async () => {
    const { user } = renderConfirmDialog(false, {
      expectedConfirmText: 'Team',
      confirmInputLabel: 'Type "Team" to confirm',
    })
    // renderWithUser resets the navigator stub, so stub after render.
    const writeText = vi.fn().mockResolvedValue(undefined)
    vi.stubGlobal('navigator', { clipboard: { writeText } })
    try {
      expect(screen.getByText('Team')).toBeVisible()
      await user.click(screen.getByRole('button', { name: 'Copy' }))
      expect(writeText).toHaveBeenCalledWith('Team')
      expect(await screen.findByText('Copied')).toBeVisible()
    } finally {
      vi.unstubAllGlobals()
    }
  })
})
