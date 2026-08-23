/* eslint-disable vue/one-component-per-file -- The transition stub reproduces PrimeVue's retained leave DOM. */
import { defineComponent, nextTick } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import { renderWithUser, screen } from '../../../test/render'
import UserAuthModals from './UserAuthModals.vue'

const mocks = vi.hoisted(() => ({
  replace: vi.fn(),
}))

vi.mock('vue-router', () => ({
  useRoute: () => ({ query: {} }),
  useRouter: () => ({ replace: mocks.replace }),
}))

vi.mock('../../../shared/composables/useSiteBranding', () => ({
  useSiteBranding: () => ({ projectName: 'IdeaBoard' }),
}))

vi.mock('../../../shared/composables/useAppToast', () => ({
  useAppToast: () => ({ success: vi.fn(), error: vi.fn() }),
}))

const TransitioningDialogStub = defineComponent({
  props: {
    visible: { type: Boolean, required: true },
  },
  data() {
    return {
      rendered: this.visible,
    }
  },
  watch: {
    visible(value: boolean) {
      if (value) {
        this.rendered = true
        return
      }
      window.setTimeout(() => {
        this.rendered = false
      }, 50)
    },
  },
  template: '<section v-if="rendered" role="dialog"><slot /></section>',
})

const AuthPanelStub = defineComponent({
  template: '<div />',
})

const OverlayLoginPanelStub = defineComponent({
  emits: ['close', 'to-register', 'login-success', 'update:busy'],
  template: `
    <div>
      <button type="button" @click="$emit('to-register')">注册</button>
      <button type="button" @click="$emit('login-success')">成功</button>
      <button type="button" @click="$emit('close')">关闭</button>
    </div>
  `,
})

describe('UserAuthModals', () => {
  it('keeps one dialog surface mounted while switching auth modes', async () => {
    const view = renderWithUser(UserAuthModals, {
      props: { mode: 'login' },
      global: {
        stubs: {
          Dialog: TransitioningDialogStub,
          LoginAuthPanel: AuthPanelStub,
          RegisterAuthPanel: AuthPanelStub,
        },
      },
    })

    expect(screen.getAllByRole('dialog')).toHaveLength(1)

    await view.rerender({ mode: 'register' })
    await nextTick()

    expect(screen.getAllByRole('dialog')).toHaveLength(1)
  })

  it('keeps the current page in place when opened as an overlay', async () => {
    const view = renderWithUser(UserAuthModals, {
      props: { mode: 'login', presentation: 'overlay' },
      global: {
        stubs: {
          Dialog: TransitioningDialogStub,
          LoginAuthPanel: OverlayLoginPanelStub,
          RegisterAuthPanel: AuthPanelStub,
        },
      },
    })

    await view.user.click(screen.getByRole('button', { name: '注册' }))
    await view.user.click(screen.getByRole('button', { name: '成功' }))
    await view.user.click(screen.getByRole('button', { name: '关闭' }))

    expect(view.emitted('update:mode')).toEqual([['register']])
    expect(view.emitted('overlay-success')).toHaveLength(1)
    expect(view.emitted('overlay-close')).toHaveLength(1)
    expect(mocks.replace).not.toHaveBeenCalled()
  })
})
