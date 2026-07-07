/* eslint-disable vue/one-component-per-file -- Local stubs keep the dialog wrapper test focused. */
import { defineComponent } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import ProfileDialog from './ProfileDialog.vue'
import { renderWithUser, screen } from '../../../test/render'

const DialogStub = defineComponent({
  props: {
    visible: { type: Boolean, required: true },
  },
  emits: ['update:visible'],
  template: `
    <section v-if="visible" role="dialog" aria-labelledby="profile-dialog-title">
      <slot name="header" />
      <slot />
      <button type="button" aria-label="关闭个人中心" @click="$emit('update:visible', false)">
        关闭
      </button>
    </section>
  `,
})

const ProfileDialogContentStub = defineComponent({
  name: 'ProfileDialogContent',
  template: '<div data-testid="profile-dialog-content">Profile content</div>',
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
})
