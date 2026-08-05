/* eslint-disable vue/one-component-per-file -- Local stubs expose public auth behavior without PrimeVue portal mechanics. */
import { defineComponent } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useUserStore } from '../../../app/stores/user'
import { renderWithUser, screen, waitFor } from '../../../test/render'
import AuthModalHost from './AuthModalHost.vue'
import LoginAuthPanel from './LoginAuthPanel.vue'
import RegisterAuthPanel from './RegisterAuthPanel.vue'

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

type AuthPanelCase = {
  action: 'login' | 'register'
  component: typeof LoginAuthPanel | typeof RegisterAuthPanel
  successEvent: 'login-success' | 'register-success'
  submitLabel: string
  switchLabel: string
  fill: (user: ReturnType<typeof renderWithUser>['user']) => Promise<void>
}

const authPanelCases: AuthPanelCase[] = [
  {
    action: 'login',
    component: LoginAuthPanel,
    successEvent: 'login-success',
    submitLabel: '登录',
    switchLabel: '立即注册',
    async fill(user) {
      await user.type(screen.getByLabelText(/^Email/), 'tester@example.com')
      await user.type(screen.getByLabelText(/^密码/), 'secret-pass')
    },
  },
  {
    action: 'register',
    component: RegisterAuthPanel,
    successEvent: 'register-success',
    submitLabel: '创建账号',
    switchLabel: '去登录',
    async fill(user) {
      await user.type(screen.getByLabelText(/^账号/), 'tester')
      await user.type(screen.getByLabelText(/^Email/), 'tester@example.com')
      await user.type(screen.getByLabelText(/^密码/), 'secret-pass')
      await user.type(screen.getByLabelText(/^确认密码/), 'secret-pass')
    },
  },
]

function stubMotionPreference(reduced: boolean) {
  vi.stubGlobal(
    'matchMedia',
    vi.fn(() => ({ matches: reduced })),
  )
}

function renderAuthHost(busy = false) {
  return renderWithUser(AuthModalHost, {
    props: { busy },
    slots: { default: '<div>Auth panel</div>' },
    global: {
      stubs: {
        Dialog: DialogStub,
      },
    },
  })
}

function renderAuthPanel(testCase: AuthPanelCase) {
  return renderWithUser(testCase.component, {
    global: {
      stubs: {
        Button: ButtonStub,
        InputText: InputTextStub,
        Message: { template: '<p><slot /></p>' },
        Password: PasswordStub,
      },
    },
  })
}

describe('AuthModalHost motion', () => {
  beforeEach(() => {
    stubMotionPreference(false)
  })

  afterEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  it('uses the Auth Motion Policy transition by default', () => {
    renderAuthHost()

    expect(screen.getByRole('dialog')).toHaveAttribute('data-transition-name', 'ib-dialog-motion')
    expect(screen.getByRole('dialog')).toHaveAttribute('data-transition-css', 'true')
  })

  it('keeps the modal visible without CSS transform motion when reduced motion is preferred', () => {
    stubMotionPreference(true)

    renderAuthHost()

    expect(screen.getByRole('dialog')).toBeVisible()
    expect(screen.getByRole('dialog')).toHaveAttribute('data-transition-css', 'false')
  })

  it('locks overlay close paths while an auth request is loading', async () => {
    const view = renderAuthHost(true)

    expect(screen.getByRole('dialog')).toHaveAttribute('data-dismissable-mask', 'false')
    expect(screen.getByRole('dialog')).toHaveAttribute('data-close-on-escape', 'false')
    expect(screen.getByRole('dialog')).toHaveAttribute('aria-busy', 'true')

    await view.user.click(screen.getByRole('button', { name: '模拟遮罩关闭' }))

    expect(view.emitted().close).toBeUndefined()
  })
})

describe.each(authPanelCases)('$action auth panel', (testCase) => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('locks controls and emits success once while the auth request is loading', async () => {
    let resolveRequest!: (value: boolean) => void
    const pendingRequest = new Promise<boolean>((resolve) => {
      resolveRequest = resolve
    })
    const store = useUserStore()
    const authAction = vi.spyOn(store, testCase.action).mockReturnValue(pendingRequest)
    const view = renderAuthPanel(testCase)

    await testCase.fill(view.user)
    const submitButton = screen.getByRole('button', { name: testCase.submitLabel })
    await view.user.click(submitButton)
    await waitFor(() => expect(authAction).toHaveBeenCalledOnce())
    await view.user.click(submitButton)

    expect(screen.getByRole('button', { name: '关闭' })).toBeDisabled()
    expect(screen.getByRole('button', { name: testCase.switchLabel })).toBeDisabled()
    expect(authAction).toHaveBeenCalledOnce()

    resolveRequest(true)
    if (testCase.action === 'register') {
      const verifyAction = vi.spyOn(store, 'verifyRegistrationOTP').mockResolvedValue(true)
      const otpButton = await screen.findByRole('button', { name: '验证并登录' })
      await view.user.type(screen.getByLabelText(/^Email 验证码/), '123456')
      await view.user.click(otpButton)
      await waitFor(() => expect(verifyAction).toHaveBeenCalledOnce())
    }
    await waitFor(() => expect(view.emitted()[testCase.successEvent]).toHaveLength(1))
  })

  it('maps validation and canonical pending state onto stable control semantics', async () => {
    let resolveRequest!: (value: boolean) => void
    const pendingRequest = new Promise<boolean>((resolve) => {
      resolveRequest = resolve
    })
    const store = useUserStore()
    vi.spyOn(store, testCase.action).mockReturnValue(pendingRequest)
    const view = renderAuthPanel(testCase)

    const submitButton = screen.getByRole('button', { name: testCase.submitLabel })
    expect(submitButton).toHaveAttribute('data-control-state', 'idle')
    expect(submitButton).not.toHaveAttribute('aria-busy')

    await view.user.click(submitButton)
    expect(screen.getByLabelText(testCase.action === 'login' ? /^Email/ : /^账号/)).toHaveAttribute(
      'data-control-tone',
      'danger',
    )

    await testCase.fill(view.user)
    await view.user.click(submitButton)

    await waitFor(() => expect(submitButton).toHaveAttribute('data-control-state', 'loading'))
    expect(submitButton).toHaveAttribute('aria-busy', 'true')
    expect(submitButton).toBeDisabled()
    expect(screen.getByLabelText(testCase.action === 'login' ? /^Email/ : /^账号/)).toHaveAttribute(
      'data-control-state',
      'disabled',
    )

    resolveRequest(true)
  })
})
