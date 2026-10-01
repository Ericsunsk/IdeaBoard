import { defineComponent } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useUserStore } from '../session/userSession'
import { renderWithUser, screen, waitFor } from '../../../test/render'
import { mockAppToast } from '../../../test/toast'
import { http, HttpResponse, useMswHandlers } from '../../../test/msw'
import AuthModalHost from './AuthModalHost.vue'
import LoginAuthPanel from './LoginAuthPanel.vue'
import RegisterAuthPanel from './RegisterAuthPanel.vue'

vi.mock('../../../shared/composables/useSiteBranding', () => ({
  useSiteBranding: () => ({ projectName: 'IdeaBoard' }),
}))

vi.mock('../../../shared/composables/useAppToast')

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
    '<div data-testid="auth-dialog-mask" v-bind="$attrs" @click="$emit(\'click\', $event)"><slot /></div>',
})

const DialogContentStub = defineComponent({
  inheritAttrs: false,
  template: `
    <section role="dialog" tabindex="-1" v-bind="$attrs" @click.stop>
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
    submitLabel: 'Sign in',
    switchLabel: 'Sign up',
    async fill(user) {
      await user.type(screen.getByLabelText(/^Email/), 'tester@example.com')
      await user.type(screen.getByLabelText(/^Password/), 'secret-pass')
    },
  },
  {
    action: 'register',
    component: RegisterAuthPanel,
    successEvent: 'register-success',
    submitLabel: 'Create account',
    switchLabel: 'Sign in',
    async fill(user) {
      await user.type(screen.getByLabelText(/^Username/), 'tester')
      await user.type(screen.getByLabelText(/^Email/), 'tester@example.com')
      await user.type(screen.getByLabelText(/^Password/), 'secret-pass')
      await user.type(screen.getByLabelText(/^Confirm Password/), 'secret-pass')
    },
  },
]

function stubMotionPreference(reduced: boolean) {
  vi.stubGlobal(
    'matchMedia',
    vi.fn(() => ({ matches: reduced })),
  )
}

function renderAuthHost(busy = false, slot = '<div>Auth panel</div>') {
  return renderWithUser(AuthModalHost, {
    props: { busy },
    slots: { default: slot },
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
}

function renderAuthPanel(testCase: AuthPanelCase) {
  useMswHandlers(
    http.get('/api/auth/capabilities', () =>
      HttpResponse.json({
        code: 0,
        msg: 'ok',
        data: { social: { google: false, discord: false }, providers: [] },
      }),
    ),
  )
  return renderWithUser(testCase.component, {
    global: {
      stubs: {
        Button: ButtonStub,
        InputText: InputTextStub,
        Message: { template: '<p><slot /></p>' },
        Password: PasswordStub,
        RouterLink: { template: '<a v-bind="$attrs"><slot /></a>' },
      },
    },
  })
}

describe('AuthModalHost motion', () => {
  beforeEach(() => {
    mockAppToast()
    stubMotionPreference(false)
  })

  afterEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  it('applies the transparent auth chrome through content class', () => {
    renderAuthHost()

    const dialog = screen.getByRole('dialog')
    expect(dialog).toHaveClass('bg-transparent')
    expect(dialog.getAttribute('class')).toContain('w-[min(390px,calc(100vw-32px))]')
  })

  it('keeps the modal visible while an auth request is loading', () => {
    renderAuthHost(true)

    expect(screen.getByRole('dialog')).toBeVisible()
  })

  it('autofocuses the first form control instead of the close action', () => {
    renderAuthHost(false, '<button type="button">Close</button><input aria-label="Email" />')

    const dialog = screen.getByRole('dialog')
    const autofocusEvent = new Event('open-auto-focus', { bubbles: true, cancelable: true })
    dialog.dispatchEvent(autofocusEvent)

    expect(autofocusEvent.defaultPrevented).toBe(true)
    expect(screen.getByRole('textbox', { name: 'Email' })).toHaveFocus()
  })

  it('dismisses from the mask while idle', async () => {
    const view = renderAuthHost(false)

    await view.user.click(screen.getByTestId('auth-dialog-mask'))
    expect(view.emitted().close).toHaveLength(1)
  })

  it('dismisses from Escape while idle', async () => {
    const view = renderAuthHost(false)

    screen.getByRole('dialog').focus()
    await view.user.keyboard('{Escape}')
    expect(view.emitted().close).toHaveLength(1)
  })

  it('locks overlay close paths while an auth request is loading', async () => {
    const view = renderAuthHost(true)

    expect(screen.getByRole('dialog')).toHaveAttribute('aria-busy', 'true')

    await view.user.click(screen.getByTestId('auth-dialog-mask'))
    expect(view.emitted().close).toBeUndefined()

    screen.getByRole('dialog').focus()
    await view.user.keyboard('{Escape}')
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

    expect(screen.getByRole('button', { name: 'Close' })).toBeDisabled()
    expect(screen.getByRole('button', { name: testCase.switchLabel })).toBeDisabled()
    expect(authAction).toHaveBeenCalledOnce()

    resolveRequest(true)
    if (testCase.action === 'register') {
      const verifyAction = vi.spyOn(store, 'verifyRegistrationOTP').mockResolvedValue(true)
      const otpButton = await screen.findByRole('button', { name: 'Verify & Sign in' })
      await view.user.type(screen.getByLabelText(/^Verification Code/), '123456')
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
    expect(
      screen.getByLabelText(testCase.action === 'login' ? /^Email/ : /^Username/),
    ).toHaveAttribute('data-control-tone', 'danger')

    await testCase.fill(view.user)
    await view.user.click(submitButton)

    await waitFor(() => expect(submitButton).toHaveAttribute('data-control-state', 'loading'))
    expect(submitButton).toHaveAttribute('aria-busy', 'true')
    expect(submitButton).toBeDisabled()
    expect(
      screen.getByLabelText(testCase.action === 'login' ? /^Email/ : /^Username/),
    ).toHaveAttribute('data-control-state', 'disabled')

    resolveRequest(true)
  })
})
