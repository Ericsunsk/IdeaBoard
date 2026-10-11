import { defineComponent } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'
import AccountDialog from './AccountDialog.vue'
import { renderWithUser, screen } from '../../../../test/render'
import { ibDialogMaskClass } from '../../../../theme/dialogMask'
import { defaultAccountTab, type AccountTab } from '../../core'

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
    '<div data-testid="account-dialog-mask" v-bind="$attrs" @click="$emit(\'click\', $event)"><slot /></div>',
})

const DialogContentStub = defineComponent({
  inheritAttrs: false,
  template: `
    <section role="dialog" tabindex="-1" v-bind="$attrs" @click.stop>
      <slot />
    </section>
  `,
})

const AccountDialogContentStub = defineComponent({
  name: 'AccountDialogContent',
  props: {
    initialTab: { type: String, default: 'profile' },
  },
  emits: ['close', 'update:busy'],
  template: `
    <div data-testid="account-dialog-content" :data-initial-tab="initialTab">
      Settings content
      <button type="button" @click="$emit('update:busy', true)">开始异步操作</button>
      <button type="button" aria-label="关闭账户管理" @click="$emit('close')">
        关闭
      </button>
    </div>
  `,
})

const StatefulAccountDialogContentStub = defineComponent({
  name: 'AccountDialogContent',
  props: {
    initialTab: { type: String, default: 'profile' },
  },
  data() {
    return {
      tab: this.initialTab,
      oldPassword: '',
    }
  },
  template: `
    <div>
      <button type="button" @click="tab = 'security'">账号安全</button>
      <section v-if="tab === 'profile'" aria-label="个人资料">个人基本资料</section>
      <label v-else>
        当前原密码
        <input v-model="oldPassword" aria-label="当前原密码" />
      </label>
    </div>
  `,
})

function renderAccountDialog(
  visible: boolean,
  onVisibleUpdate = vi.fn(),
  initialTab: AccountTab = defaultAccountTab,
) {
  return {
    onVisibleUpdate,
    ...renderWithUser(AccountDialog, {
      props: {
        visible,
        initialTab,
        'onUpdate:visible': onVisibleUpdate,
      },
      global: {
        stubs: {
          DialogRoot: DialogRootStub,
          DialogPortal: PassthroughStub,
          DialogOverlay: DialogOverlayStub,
          DialogContent: DialogContentStub,
          DialogDescription: PassthroughStub,
          DialogTitle: PassthroughStub,
          AccountDialogContent: AccountDialogContentStub,
        },
      },
    }),
  }
}

function renderAccountDialogWithStatefulContent(visible: boolean) {
  return renderWithUser(AccountDialog, {
    props: {
      visible,
    },
    global: {
      stubs: {
        DialogRoot: DialogRootStub,
        DialogPortal: PassthroughStub,
        DialogOverlay: PassthroughStub,
        DialogContent: DialogContentStub,
        DialogDescription: PassthroughStub,
        DialogTitle: PassthroughStub,
        AccountDialogContent: StatefulAccountDialogContentStub,
      },
    },
  })
}

describe('AccountDialog', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('applies the shared overlay chrome through content class', () => {
    renderAccountDialog(true)

    expect(screen.getByRole('dialog', { name: 'Account' })).toHaveClass('shadow-xl')
  })

  it('reuses the shared dialog mask token for the overlay veil (no dialog-specific overlay token)', () => {
    expect(ibDialogMaskClass).toContain('var(--color-dialog-mask)')

    renderAccountDialog(true)

    expect(screen.getByTestId('account-dialog-mask')).toHaveClass('!bg-[var(--color-dialog-mask)]')
  })

  it('renders dialog-ready account management content when visible', () => {
    renderAccountDialog(true)

    expect(screen.getByRole('dialog', { name: 'Account' })).toBeVisible()
    expect(screen.getByTestId('account-dialog-content')).toBeVisible()
  })

  it('does not mount account management content while hidden', () => {
    renderAccountDialog(false)

    expect(screen.queryByTestId('account-dialog-content')).not.toBeInTheDocument()
  })

  it('forwards the requested initial tab to dialog content', () => {
    renderAccountDialog(true, vi.fn(), 'billing')

    expect(screen.getByTestId('account-dialog-content')).toHaveAttribute(
      'data-initial-tab',
      'billing',
    )
  })

  it('emits visibility updates without owning route state', async () => {
    const { user, onVisibleUpdate } = renderAccountDialog(true)

    await user.click(screen.getByRole('button', { name: '关闭账户管理' }))

    expect(onVisibleUpdate).toHaveBeenCalledWith(false)
  })

  it('closes from the mask, Escape, and the close button while idle', async () => {
    const { user, onVisibleUpdate } = renderAccountDialog(true)

    await user.click(screen.getByTestId('account-dialog-mask'))
    expect(onVisibleUpdate).toHaveBeenCalledWith(false)
  })

  it('closes from Escape and the close button', async () => {
    const { user, onVisibleUpdate } = renderAccountDialog(true)
    const dialog = screen.getByRole('dialog', { name: 'Account' })

    dialog.focus()
    await user.keyboard('{Escape}')
    expect(onVisibleUpdate).toHaveBeenCalledWith(false)
  })

  it('lets canonical pending state lock the mask, Escape, and the content close control', async () => {
    const { user, onVisibleUpdate } = renderAccountDialog(true)
    const dialog = screen.getByRole('dialog', { name: 'Account' })

    await user.click(screen.getByRole('button', { name: '开始异步操作' }))
    expect(dialog).toHaveAttribute('aria-busy', 'true')

    await user.click(screen.getByTestId('account-dialog-mask'))
    expect(onVisibleUpdate).not.toHaveBeenCalled()

    await user.click(screen.getByRole('button', { name: '关闭账户管理' }))
    dialog.focus()
    await user.keyboard('{Escape}')

    expect(onVisibleUpdate).not.toHaveBeenCalled()
  })

  it('unmounts panel state so reopening starts from profile without stale password input', async () => {
    const { user, rerender } = renderAccountDialogWithStatefulContent(true)

    await user.click(screen.getByRole('button', { name: '账号安全' }))
    await user.type(screen.getByRole('textbox', { name: '当前原密码' }), 'old-secret')
    expect(screen.getByRole('textbox', { name: '当前原密码' })).toHaveValue('old-secret')

    await rerender({ visible: false })
    expect(screen.queryByRole('textbox', { name: '当前原密码' })).not.toBeInTheDocument()

    await rerender({ visible: true })
    expect(screen.getByRole('region', { name: '个人资料' })).toBeVisible()
    expect(screen.queryByRole('textbox', { name: '当前原密码' })).not.toBeInTheDocument()
  })
})
