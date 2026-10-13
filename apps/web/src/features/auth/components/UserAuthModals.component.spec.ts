import { defineComponent, nextTick } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { renderWithUser, screen } from '../../../test/render'
import { mockAppToast } from '../../../test/toast'
import UserAuthModals from './UserAuthModals.vue'

const mocks = vi.hoisted(() => ({
  replace: vi.fn(),
  query: {} as Record<string, string>,
}))

vi.mock('vue-router', () => ({
  useRoute: () => ({ query: mocks.query }),
  useRouter: () => ({ replace: mocks.replace }),
}))

vi.mock('../../../core/composables/useSiteBranding', () => ({
  useSiteBranding: () => ({ projectName: 'IdeaBoard' }),
}))

vi.mock('../../../core/composables/useAppToast')

const TransitioningDialogRootStub = defineComponent({
  props: {
    open: { type: Boolean, required: true },
  },
  emits: ['update:open'],
  data() {
    return {
      rendered: this.open,
    }
  },
  watch: {
    open(value: boolean) {
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

const PassthroughStub = defineComponent({
  template: '<slot />',
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
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.query = {}
    mockAppToast()
  })

  it('keeps one dialog surface mounted while switching auth modes', async () => {
    const view = renderWithUser(UserAuthModals, {
      props: { mode: 'login' },
      global: {
        stubs: {
          DialogRoot: TransitioningDialogRootStub,
          DialogPortal: PassthroughStub,
          DialogOverlay: PassthroughStub,
          DialogContent: PassthroughStub,
          DialogDescription: PassthroughStub,
          DialogTitle: PassthroughStub,
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
          DialogRoot: TransitioningDialogRootStub,
          DialogPortal: PassthroughStub,
          DialogOverlay: PassthroughStub,
          DialogContent: PassthroughStub,
          DialogDescription: PassthroughStub,
          DialogTitle: PassthroughStub,
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

  it.each([
    { action: '成功', destination: '/admin/users?page=1#active' },
    { action: '关闭', destination: '/' },
  ])(
    'navigates to $destination on $action with an Admin redirect',
    async ({ action, destination }) => {
      mocks.query = { redirect: '/admin/users?page=1#active' }
      const view = renderWithUser(UserAuthModals, {
        props: { mode: 'login' },
        global: {
          stubs: {
            DialogRoot: TransitioningDialogRootStub,
            DialogPortal: PassthroughStub,
            DialogOverlay: PassthroughStub,
            DialogContent: PassthroughStub,
            DialogDescription: PassthroughStub,
            DialogTitle: PassthroughStub,
            LoginAuthPanel: OverlayLoginPanelStub,
            RegisterAuthPanel: AuthPanelStub,
          },
        },
      })

      await view.user.click(screen.getByRole('button', { name: action }))

      expect(mocks.replace).toHaveBeenCalledWith(destination)
    },
  )
})
