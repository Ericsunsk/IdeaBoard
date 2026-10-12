import { defineComponent } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent } from '@testing-library/vue'
import { useUserStore } from '../session/userSession'
import { renderWithUser, screen, waitFor } from '../../../test/render'
import { mockAppToast } from '../../../test/toast'
import { http, HttpResponse, useMswHandlers } from '../../../test/msw'
import { createDeferred } from '../../../test/deferred'
import LoginAuthPanel from './LoginAuthPanel.vue'
import RegisterAuthPanel from './RegisterAuthPanel.vue'

vi.mock('../../../core/composables/useSiteBranding', () => ({
  useSiteBranding: () => ({ projectName: 'IdeaBoard' }),
}))

vi.mock('../../../core/composables/useAppToast')

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

const authPanelStubs = {
  Button: ButtonStub,
  RouterLink: { template: '<a v-bind="$attrs"><slot /></a>' },
}

function renderLoginPanel() {
  useMswHandlers(
    http.get('/api/auth/capabilities', () =>
      HttpResponse.json({
        code: 0,
        msg: 'ok',
        data: {
          social: { google: false, discord: false } satisfies Record<string, boolean>,
          providers: [],
        },
      }),
    ),
  )
  return renderWithUser(LoginAuthPanel, {
    global: {
      stubs: authPanelStubs,
    },
  })
}

function renderRegisterPanel() {
  return renderWithUser(RegisterAuthPanel, {
    global: {
      stubs: authPanelStubs,
    },
  })
}

describe('auth save pending gate', () => {
  beforeEach(() => {
    mockAppToast()
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('blocks a second auth save while login isPending', async () => {
    const loginDeferred = createDeferred<boolean>()
    const store = useUserStore()
    const loginSpy = vi.spyOn(store, 'login').mockReturnValue(loginDeferred.promise)
    const view = renderLoginPanel()

    await view.user.type(screen.getByLabelText(/^Email/), 'tester@example.com')
    const passwordInput = screen.getByLabelText(/^Password/)
    await view.user.type(passwordInput, 'secret-pass')
    await view.user.click(screen.getByRole('button', { name: 'Sign in' }))
    await waitFor(() => expect(loginSpy).toHaveBeenCalledOnce())
    expect(screen.queryAllByRole('alert')).toHaveLength(0)

    // Bypass the disabled state to force a second submit while isPending.
    // The isPending gate must return before re-running validation.
    await fireEvent.update(passwordInput, '')
    const loginForm = view.container.querySelector('form')
    if (!loginForm) throw new Error('login form is missing')
    await fireEvent.submit(loginForm)

    expect(loginSpy).toHaveBeenCalledOnce()
    expect(screen.queryAllByRole('alert')).toHaveLength(0)

    loginDeferred.resolve(true)
    await waitFor(() => expect(view.emitted()['login-success']).toHaveLength(1))
  })

  it('makes a failed login retryable once isPending settles', async () => {
    const store = useUserStore()
    const loginSpy = vi
      .spyOn(store, 'login')
      .mockRejectedValueOnce(new Error('boom'))
      .mockResolvedValueOnce(true)
    const view = renderLoginPanel()

    await view.user.type(screen.getByLabelText(/^Email/), 'tester@example.com')
    await view.user.type(screen.getByLabelText(/^Password/), 'secret-pass')
    const submit = screen.getByRole('button', { name: 'Sign in' })
    await view.user.click(submit)
    await waitFor(() => expect(loginSpy).toHaveBeenCalledTimes(1))
    await waitFor(() => expect(submit).toHaveAttribute('data-control-state', 'idle'))
    expect(submit).not.toBeDisabled()

    await view.user.click(submit)
    await waitFor(() => expect(loginSpy).toHaveBeenCalledTimes(2))
    await waitFor(() => expect(view.emitted()['login-success']).toHaveLength(1))
  })

  it('blocks double register submit while isPending', async () => {
    const registerDeferred = createDeferred<boolean>()
    const store = useUserStore()
    const registerSpy = vi.spyOn(store, 'register').mockReturnValue(registerDeferred.promise)
    const view = renderRegisterPanel()

    await view.user.type(screen.getByLabelText(/^Username/), 'tester')
    await view.user.type(screen.getByLabelText(/^Email/), 'tester@example.com')
    const passwords = screen.getAllByLabelText(/Password/)
    const passwordInput = passwords[0]
    if (!passwordInput) throw new Error('password field is missing')
    await view.user.type(passwords[0]!, 'secret-pass-1')
    await view.user.type(passwords[1]!, 'secret-pass-1')
    await view.user.click(screen.getByRole('button', { name: 'Create account' }))
    await waitFor(() => expect(registerSpy).toHaveBeenCalledOnce())
    expect(screen.queryAllByRole('alert')).toHaveLength(0)

    // Bypass the disabled state: a second submit while isPending must not
    // re-run validation nor issue a second save.
    await fireEvent.update(passwordInput, '')
    const registerForm = view.container.querySelector('form')
    if (!registerForm) throw new Error('register form is missing')
    await fireEvent.submit(registerForm)

    expect(registerSpy).toHaveBeenCalledOnce()
    expect(screen.queryAllByRole('alert')).toHaveLength(0)

    registerDeferred.resolve(true)
    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'Verify & Sign in' })).toBeVisible(),
    )
  })
})
