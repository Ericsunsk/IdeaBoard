/* eslint-disable vue/one-component-per-file -- Local stubs keep the dialog wrapper test focused. */
import { defineComponent } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import AccountManagementDialog from './AccountManagementDialog.vue'
import { renderWithUser, screen } from '../../../test/render'

const DialogStub = defineComponent({
  inheritAttrs: false,
  props: {
    visible: { type: Boolean, required: true },
    dismissableMask: { type: Boolean, default: true },
    closeOnEscape: { type: Boolean, default: false },
    showHeader: { type: Boolean, default: true },
  },
  emits: ['update:visible'],
  template: `
    <div
      v-if="visible"
      data-testid="account-management-dialog-mask"
      :data-dismissable-mask="String(dismissableMask)"
      @click="dismissableMask && $emit('update:visible', false)"
    >
      <section
        role="dialog"
        tabindex="-1"
        :data-close-on-escape="String(closeOnEscape)"
        :data-show-header="String(showHeader)"
        @click.stop
        @keydown.escape="closeOnEscape && $emit('update:visible', false)"
        v-bind="$attrs"
      >
        <slot v-if="showHeader" name="header" />
        <slot />
      </section>
    </div>
  `,
})

const AccountManagementDialogContentStub = defineComponent({
  name: 'AccountManagementDialogContent',
  props: {
    initialTab: { type: String, default: 'profile' },
  },
  emits: ['close'],
  template: `
    <div data-testid="account-management-dialog-content" :data-initial-tab="initialTab">
      Account management content
      <button type="button" aria-label="关闭账户管理" @click="$emit('close')">
        关闭
      </button>
    </div>
  `,
})

const StatefulAccountManagementDialogContentStub = defineComponent({
  name: 'AccountManagementDialogContent',
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

function renderAccountManagementDialog(
  visible: boolean,
  onVisibleUpdate = vi.fn(),
  initialTab: 'profile' | 'credits' | 'security' = 'profile',
) {
  return {
    onVisibleUpdate,
    ...renderWithUser(AccountManagementDialog, {
      props: {
        visible,
        initialTab,
        'onUpdate:visible': onVisibleUpdate,
      },
      global: {
        stubs: {
          Dialog: DialogStub,
          AccountManagementDialogContent: AccountManagementDialogContentStub,
        },
      },
    }),
  }
}

function renderAccountManagementDialogWithStatefulContent(visible: boolean) {
  return renderWithUser(AccountManagementDialog, {
    props: {
      visible,
    },
    global: {
      stubs: {
        Dialog: DialogStub,
        AccountManagementDialogContent: StatefulAccountManagementDialogContentStub,
      },
    },
  })
}

describe('AccountManagementDialog', () => {
  it('renders dialog-ready account management content when visible', () => {
    renderAccountManagementDialog(true)

    expect(screen.getByRole('dialog', { name: '账户管理' })).toBeVisible()
    expect(screen.getByRole('dialog', { name: '账户管理' })).toHaveAttribute(
      'data-show-header',
      'false',
    )
    expect(screen.getByTestId('account-management-dialog-content')).toBeVisible()
  })

  it('does not mount account management content while hidden', () => {
    renderAccountManagementDialog(false)

    expect(screen.queryByTestId('account-management-dialog-content')).not.toBeInTheDocument()
  })

  it('forwards the requested initial tab to dialog content', () => {
    renderAccountManagementDialog(true, vi.fn(), 'credits')

    expect(screen.getByTestId('account-management-dialog-content')).toHaveAttribute(
      'data-initial-tab',
      'credits',
    )
  })

  it('emits visibility updates without owning route state', async () => {
    const { user, onVisibleUpdate } = renderAccountManagementDialog(true)

    await user.click(screen.getByRole('button', { name: '关闭账户管理' }))

    expect(onVisibleUpdate).toHaveBeenCalledWith(false)
  })

  it('closes from Escape and the close button, but not from the mask', async () => {
    const { user, onVisibleUpdate } = renderAccountManagementDialog(true)
    const dialog = screen.getByRole('dialog', { name: '账户管理' })

    expect(screen.getByTestId('account-management-dialog-mask')).toHaveAttribute(
      'data-dismissable-mask',
      'false',
    )
    expect(dialog).toHaveAttribute('data-close-on-escape', 'true')

    await user.click(screen.getByTestId('account-management-dialog-mask'))
    expect(onVisibleUpdate).not.toHaveBeenCalled()

    dialog.focus()
    await user.keyboard('{Escape}')
    expect(onVisibleUpdate).toHaveBeenCalledWith(false)
  })

  it('unmounts panel state so reopening starts from profile without stale password input', async () => {
    const { user, rerender } = renderAccountManagementDialogWithStatefulContent(true)

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
