/* eslint-disable vue/one-component-per-file -- Local stubs keep the dialog wrapper test focused. */
import { defineComponent } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import ProfileDialog from './ProfileDialog.vue'
import { renderWithUser, screen } from '../../../test/render'

const DialogStub = defineComponent({
  props: {
    visible: { type: Boolean, required: true },
    dismissableMask: { type: Boolean, default: true },
    closeOnEscape: { type: Boolean, default: false },
  },
  emits: ['update:visible'],
  template: `
    <div
      v-if="visible"
      data-testid="profile-dialog-mask"
      :data-dismissable-mask="String(dismissableMask)"
      @click="dismissableMask && $emit('update:visible', false)"
    >
      <section
        role="dialog"
        aria-labelledby="profile-dialog-title"
        tabindex="-1"
        :data-close-on-escape="String(closeOnEscape)"
        @click.stop
        @keydown.escape="closeOnEscape && $emit('update:visible', false)"
      >
        <slot name="header" />
        <slot />
        <button type="button" aria-label="关闭个人中心" @click="$emit('update:visible', false)">
          关闭
        </button>
      </section>
    </div>
  `,
})

const ProfileDialogContentStub = defineComponent({
  name: 'ProfileDialogContent',
  template: '<div data-testid="profile-dialog-content">Profile content</div>',
})

const StatefulProfileDialogContentStub = defineComponent({
  name: 'ProfileDialogContent',
  data() {
    return {
      tab: 'profile',
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

function renderProfileDialog(visible: boolean, onVisibleUpdate = vi.fn()) {
  return {
    onVisibleUpdate,
    ...renderWithUser(ProfileDialog, {
      props: {
        visible,
        'onUpdate:visible': onVisibleUpdate,
      },
      global: {
        stubs: {
          Dialog: DialogStub,
          ProfileDialogContent: ProfileDialogContentStub,
        },
      },
    }),
  }
}

function renderProfileDialogWithStatefulContent(visible: boolean) {
  return renderWithUser(ProfileDialog, {
    props: {
      visible,
    },
    global: {
      stubs: {
        Dialog: DialogStub,
        ProfileDialogContent: StatefulProfileDialogContentStub,
      },
    },
  })
}

describe('ProfileDialog', () => {
  it('renders dialog-ready profile content when visible', () => {
    renderProfileDialog(true)

    expect(screen.getByRole('dialog', { name: '个人中心' })).toBeVisible()
    expect(screen.getByTestId('profile-dialog-content')).toBeVisible()
  })

  it('does not mount profile content while hidden', () => {
    renderProfileDialog(false)

    expect(screen.queryByTestId('profile-dialog-content')).not.toBeInTheDocument()
  })

  it('emits visibility updates without owning route state', async () => {
    const { user, onVisibleUpdate } = renderProfileDialog(true)

    await user.click(screen.getByRole('button', { name: '关闭个人中心' }))

    expect(onVisibleUpdate).toHaveBeenCalledWith(false)
  })

  it('closes from Escape and the close button, but not from the mask', async () => {
    const { user, onVisibleUpdate } = renderProfileDialog(true)
    const dialog = screen.getByRole('dialog', { name: '个人中心' })

    expect(screen.getByTestId('profile-dialog-mask')).toHaveAttribute(
      'data-dismissable-mask',
      'false',
    )
    expect(dialog).toHaveAttribute('data-close-on-escape', 'true')

    await user.click(screen.getByTestId('profile-dialog-mask'))
    expect(onVisibleUpdate).not.toHaveBeenCalled()

    dialog.focus()
    await user.keyboard('{Escape}')
    expect(onVisibleUpdate).toHaveBeenCalledWith(false)
  })

  it('unmounts panel state so reopening starts from profile without stale password input', async () => {
    const { user, rerender } = renderProfileDialogWithStatefulContent(true)

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
