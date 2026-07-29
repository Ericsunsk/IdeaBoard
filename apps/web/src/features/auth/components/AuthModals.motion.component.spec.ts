/* eslint-disable vue/one-component-per-file -- Local stubs expose public modal behavior without PrimeVue portal mechanics. */
import { defineComponent } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useUserStore } from '../../../app/stores/user'
import { renderWithUser, screen, waitFor } from '../../../test/render'
import LoginModal from './LoginModal.vue'
import RegisterModal from './RegisterModal.vue'

vi.mock('../../../shared/composables/useSiteBranding', () => ({
  useSiteBranding: () => ({ projectName: 'IdeaBoard' }),
}))

vi.mock('../../../shared/composables/useAppToast', () => ({
  useAppToast: () => ({ success: vi.fn(), error: vi.fn() }),
}))

const DialogStub = defineComponent({
  inheritAttrs: false,
  props: {
    visible: { type: Boolean, required: true },
    dismissableMask: { type: Boolean, default: true },
    closeOnEscape: { type: Boolean, default: true },
    pt: { type: Object, default: () => ({}) },
  },
  emits: ['update:visible'],
  computed: {
    transitionName(): string {
      return String((this.pt as { transition?: { name?: unknown } }).transition?.name ?? '')
    },
    transitionCss(): string {
      return String((this.pt as { transition?: { css?: unknown } }).transition?.css ?? '')
    },
  },
  template: `
    <section
      v-if="visible"
      v-bind="$attrs"
      role="dialog"
      :data-dismissable-mask="String(dismissableMask)"
      :data-close-on-escape="String(closeOnEscape)"
      :data-transition-name="transitionName"
      :data-transition-css="transitionCss"
    >
      <button type="button" aria-label="模拟遮罩关闭" @click="$emit('update:visible', false)">
        模拟遮罩关闭
      </button>
      <slot />
    </section>
  `,
})

const ButtonStub = defineComponent({
  inheritAttrs: false,
  props: {
    disabled: { type: Boolean, default: false },
    label: { type: String, default: '' },
    type: { type: String, default: 'button' },
  },
  emits: ['click'],
  template: `
    <button
      v-bind="$attrs"
      :type="type"
      :disabled="disabled"
      @click="$emit('click', $event)"
    >
      <slot name="icon" />
      <slot />
      {{ label }}
    </button>
  `,
})

const InputTextStub = defineComponent({
  inheritAttrs: false,
  props: {
    id: { type: String, default: '' },
    modelValue: { type: String, default: '' },
  },
  emits: ['update:modelValue', 'blur'],
  methods: {
    handleInput(event: Event) {
      this.$emit('update:modelValue', (event.target as HTMLInputElement).value)
    },
  },
  template: `
    <input
      v-bind="$attrs"
      :id="id"
      :value="modelValue"
      @input="handleInput"
      @blur="$emit('blur')"
    />
  `,
})

const PasswordStub = defineComponent({
  inheritAttrs: false,
  props: {
    inputProps: { type: Object, default: () => ({}) },
    modelValue: { type: String, default: '' },
  },
  emits: ['update:modelValue', 'blur'],
  methods: {
    handleInput(event: Event) {
      this.$emit('update:modelValue', (event.target as HTMLInputElement).value)
    },
  },
  template: `
    <input
      v-bind="inputProps"
      type="password"
      :value="modelValue"
      @input="handleInput"
      @blur="$emit('blur')"
    />
  `,
})

type AuthModalCase = {
  action: 'login' | 'register'
  component: typeof LoginModal | typeof RegisterModal
  submitLabel: string
  switchLabel: string
  fill: (user: ReturnType<typeof renderWithUser>['user']) => Promise<void>
}

const authModalCases: AuthModalCase[] = [
  {
    action: 'login',
    component: LoginModal,
    submitLabel: '登录',
    switchLabel: '立即注册',
    async fill(user) {
      await user.type(screen.getByLabelText(/^账号/), 'tester')
      await user.type(screen.getByLabelText(/^密码/), 'secret')
    },
  },
  {
    action: 'register',
    component: RegisterModal,
    submitLabel: '创建账号',
    switchLabel: '去登录',
    async fill(user) {
      await user.type(screen.getByLabelText(/^账号/), 'tester')
      await user.type(screen.getByLabelText(/^密码/), 'secret')
      await user.type(screen.getByLabelText(/^确认密码/), 'secret')
    },
  },
]

function stubMotionPreference(reduced: boolean) {
  vi.stubGlobal(
    'matchMedia',
    vi.fn(() => ({ matches: reduced })),
  )
}

function renderAuthModal(testCase: AuthModalCase) {
  return renderWithUser(testCase.component, {
    props: { show: true },
    global: {
      stubs: {
        Button: ButtonStub,
        Dialog: DialogStub,
        InputText: InputTextStub,
        Message: { template: '<p><slot /></p>' },
        Password: PasswordStub,
      },
    },
  })
}

describe.each(authModalCases)('$action modal motion', (testCase) => {
  beforeEach(() => {
    stubMotionPreference(false)
  })

  afterEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  it('uses the Auth Motion Policy transition by default', () => {
    renderAuthModal(testCase)

    expect(screen.getByRole('dialog')).toHaveAttribute('data-transition-name', 'ib-dialog-motion')
    expect(screen.getByRole('dialog')).toHaveAttribute('data-transition-css', 'true')
  })

  it('keeps the modal visible without CSS transform motion when reduced motion is preferred', () => {
    stubMotionPreference(true)

    renderAuthModal(testCase)

    expect(screen.getByRole('dialog')).toBeVisible()
    expect(screen.getByRole('dialog')).toHaveAttribute('data-transition-css', 'false')
  })

  it('locks every close path while the auth request is loading', async () => {
    let resolveRequest!: (value: boolean) => void
    const pendingRequest = new Promise<boolean>((resolve) => {
      resolveRequest = resolve
    })
    const store = useUserStore()
    const authAction = vi.spyOn(store, testCase.action).mockReturnValue(pendingRequest)
    const view = renderAuthModal(testCase)

    await testCase.fill(view.user)
    const submitButton = screen.getByRole('button', { name: testCase.submitLabel })
    await view.user.click(submitButton)
    await waitFor(() => expect(authAction).toHaveBeenCalledOnce())
    await view.user.click(submitButton)

    expect(screen.getByRole('dialog')).toHaveAttribute('data-dismissable-mask', 'false')
    expect(screen.getByRole('dialog')).toHaveAttribute('data-close-on-escape', 'false')
    expect(screen.getByRole('dialog')).toHaveAttribute('aria-busy', 'true')
    expect(screen.getByRole('button', { name: '关闭' })).toBeDisabled()
    expect(screen.getByRole('button', { name: testCase.switchLabel })).toBeDisabled()

    await view.user.click(screen.getByRole('button', { name: '模拟遮罩关闭' }))
    expect(view.emitted()['update:show']).toBeUndefined()
    expect(authAction).toHaveBeenCalledOnce()

    resolveRequest(true)
    await waitFor(() => expect(view.emitted()['update:show']).toContainEqual([false]))
  })

  it('maps validation and canonical pending state onto stable control semantics', async () => {
    let resolveRequest!: (value: boolean) => void
    const pendingRequest = new Promise<boolean>((resolve) => {
      resolveRequest = resolve
    })
    const store = useUserStore()
    vi.spyOn(store, testCase.action).mockReturnValue(pendingRequest)
    const view = renderAuthModal(testCase)

    const submitButton = screen.getByRole('button', { name: testCase.submitLabel })
    expect(submitButton).toHaveAttribute('data-control-state', 'idle')
    expect(submitButton).not.toHaveAttribute('aria-busy')

    await view.user.click(submitButton)
    expect(screen.getByLabelText(/^账号/)).toHaveAttribute('data-control-tone', 'danger')

    await testCase.fill(view.user)
    await view.user.click(submitButton)

    await waitFor(() => expect(submitButton).toHaveAttribute('data-control-state', 'loading'))
    expect(submitButton).toHaveAttribute('aria-busy', 'true')
    expect(submitButton).toBeDisabled()
    expect(screen.getByLabelText(/^账号/)).toHaveAttribute('data-control-state', 'disabled')

    resolveRequest(true)
  })
})
