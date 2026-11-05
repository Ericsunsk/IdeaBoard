import { defineComponent } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import ConfirmDialog from './ConfirmDialog.vue'
import { activeConfirm, useConfirm } from '../composables/useConfirmDialog'
import { renderWithUser, screen } from '../../test/render'
import { setUserLocale } from '../i18n'

const toast = vi.hoisted(() => ({ error: vi.fn() }))

vi.mock('../composables/useAppToast', () => ({
  useAppToast: () => toast,
}))

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
    '<div data-testid="confirm-mask" v-bind="$attrs" @click="$emit(\'click\', $event)"><slot /></div>',
})

const DialogContentStub = defineComponent({
  inheritAttrs: false,
  template: `
    <section role="dialog" tabindex="-1" v-bind="$attrs" @click.stop>
      <slot />
    </section>
  `,
})

function renderUserDialog(isLoading = false, extraProps: Record<string, unknown> = {}) {
  const onVisibleUpdate = vi.fn()
  const onConfirm = vi.fn()
  const view = renderWithUser(ConfirmDialog, {
    props: {
      surface: 'user',
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

describe('ConfirmDialog', () => {
  afterEach(async () => {
    await setUserLocale('en')
  })

  beforeEach(() => {
    activeConfirm.value = null
    vi.clearAllMocks()
  })

  it.each(['en', 'es', 'ko'] as const)(
    'keeps admin cancel and busy copy Chinese under %s',
    async (locale) => {
      await setUserLocale(locale)
      renderWithUser(ConfirmDialog, {
        props: { surface: 'admin', isVisible: true, title: '确认删除', isLoading: true },
      })

      expect(await screen.findByRole('button', { name: '取消' })).toBeDisabled()
      expect(screen.getByRole('button', { name: '处理中...' })).toBeDisabled()
    },
  )

  it.each([
    ['en', 'Cancel', 'Confirm'],
    ['es', 'Cancelar', 'Confirmar'],
    ['ko', '취소', '확인'],
  ] as const)('keeps user controls translated under %s', async (locale, cancel, confirm) => {
    await setUserLocale(locale)
    renderUserDialog(false, { confirmLabel: '' })

    expect(screen.getByRole('button', { name: cancel })).toBeVisible()
    expect(screen.getByRole('button', { name: confirm })).toBeVisible()
  })

  it('keeps the admin failure fallback Chinese under a non-Chinese locale', async () => {
    await setUserLocale('en')
    const error = new Error('dial tcp 127.0.0.1:5432: connection refused')
    const rejectedConfirm = useConfirm()
      .confirm({
        title: '确认删除',
        message: '删除后不可恢复。',
        confirmLabel: '删除',
        intent: 'danger',
        onConfirm: async () => {
          throw error
        },
      })
      .catch((reason: unknown) => reason)
    const { user } = renderWithUser(ConfirmDialog, { props: { surface: 'admin' } })

    await user.click(await screen.findByRole('button', { name: '删除' }))

    await expect(rejectedConfirm).resolves.toBe(error)
    expect(toast.error).toHaveBeenCalledWith('操作失败，请重试')
  })

  it('dismisses from the mask through the shared lifecycle while idle', async () => {
    const { user, onVisibleUpdate } = renderUserDialog()

    await user.click(screen.getByTestId('confirm-mask'))
    expect(onVisibleUpdate).toHaveBeenCalledWith(false)
  })

  it('dismisses from Escape through the shared lifecycle while idle', async () => {
    const { user, onVisibleUpdate } = renderUserDialog()

    screen.getByRole('dialog', { name: '删除确认' }).focus()
    await user.keyboard('{Escape}')
    expect(onVisibleUpdate).toHaveBeenCalledWith(false)
  })

  it('emits confirm and cancel through its own controls while idle', async () => {
    const { user, onVisibleUpdate, onConfirm } = renderUserDialog()

    await user.click(screen.getByRole('button', { name: '删除' }))
    expect(onConfirm).toHaveBeenCalledTimes(1)

    await user.click(screen.getByRole('button', { name: /cancel|取消/i }))
    expect(onVisibleUpdate).toHaveBeenCalledWith(false)
  })

  it('locks mask, Escape, and external close while the irreversible commit is busy', async () => {
    const { user, onVisibleUpdate } = renderUserDialog(true)
    const dialog = screen.getByRole('dialog', { name: '删除确认' })

    expect(dialog).toHaveAttribute('aria-busy', 'true')

    await user.click(screen.getByTestId('confirm-mask'))
    expect(onVisibleUpdate).not.toHaveBeenCalled()

    dialog.focus()
    await user.keyboard('{Escape}')
    expect(onVisibleUpdate).not.toHaveBeenCalled()
  })

  it('disables confirm until the typed text matches the expected name', async () => {
    const { user, onConfirm } = renderUserDialog(false, {
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
    const { user, onConfirm } = renderUserDialog(false, {
      expectedConfirmText: 'Team',
      confirmInputLabel: 'Type "Team" to confirm',
    })

    const nameInput = screen.getByLabelText('Type "Team" to confirm')
    await user.type(nameInput, 'Team')
    await user.keyboard('{Enter}')
    expect(onConfirm).toHaveBeenCalledTimes(1)
  })

  it('disables the typed input and confirm while the commit is busy', async () => {
    const { user, onConfirm } = renderUserDialog(true, {
      expectedConfirmText: 'Team',
      confirmInputLabel: 'Type "Team" to confirm',
    })

    const nameInput = screen.getByLabelText('Type "Team" to confirm')
    expect(nameInput).toBeDisabled()
    expect(screen.getByRole('button', { name: '删除' })).toBeDisabled()

    await user.keyboard('{Enter}')
    expect(onConfirm).not.toHaveBeenCalled()
  })

  it('renders the typed hint as plain text with no copy affordance', async () => {
    renderUserDialog(false, {
      expectedConfirmText: 'Team',
      confirmInputLabel: 'Type "Team" to confirm',
    })

    expect(screen.getByText('Type "Team" to confirm')).toBeVisible()
    expect(screen.queryByRole('button', { name: 'Copy' })).toBeNull()
    expect(screen.getByLabelText('Type "Team" to confirm')).toBeVisible()
  })

  it('shows a safe error and keeps the admin dialog open when confirmation fails', async () => {
    const error = new Error('operation failed')
    const confirmPromise = useConfirm().confirm({
      title: '确认删除',
      message: '删除后不可恢复。',
      confirmLabel: '删除',
      intent: 'danger',
      onConfirm: async () => {
        throw error
      },
    })
    const rejectedConfirm = confirmPromise.catch((reason: unknown) => reason)
    const { user } = renderWithUser(ConfirmDialog, { props: { surface: 'admin' } })

    expect(await screen.findByRole('alertdialog')).toHaveClass('bg-popover')
    await user.click(await screen.findByRole('button', { name: '删除' }))

    await expect(rejectedConfirm).resolves.toBe(error)
    expect(toast.error).toHaveBeenCalledWith('operation failed')
    expect(activeConfirm.value?.open).toBe(true)
    expect(activeConfirm.value?.loading).toBe(false)
  })
})
