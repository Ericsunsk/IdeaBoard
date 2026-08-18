/* eslint-disable vue/one-component-per-file -- Small local stubs keep OAuth behavior tests independent of PrimeVue internals. */
import { defineComponent, ref } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { renderWithUser, screen, waitFor } from '../../../test/render'
import AdminLoginPage from '../../admin/pages/login/AdminLoginPage.vue'
import LoginAuthPanel from '../components/LoginAuthPanel.vue'
import UserAuthCallbackPage from './UserAuthCallbackPage.vue'

const mocks = vi.hoisted(() => ({
  route: {
    path: '/user/auth/callback',
    query: {} as Record<string, string | string[] | undefined>,
  },
  routerPush: vi.fn(),
  routerReplace: vi.fn(),
  ensureSession: vi.fn(),
  clearAuth: vi.fn(),
  login: vi.fn(),
  logout: vi.fn(),
  getCapabilities: vi.fn(),
  signInWithOAuth: vi.fn(),
  toastError: vi.fn(),
  toastSuccess: vi.fn(),
}))

vi.mock('vue-router', () => ({
  useRoute: () => mocks.route,
  useRouter: () => ({ push: mocks.routerPush, replace: mocks.routerReplace }),
}))

vi.mock('../session/authSession', () => ({
  useUserStore: () => ({
    isAdmin: false,
    login: mocks.login,
    logout: mocks.logout,
    ensureSession: mocks.ensureSession,
    clearAuth: mocks.clearAuth,
  }),
}))

vi.mock('../session/authTransport', () => ({
  authTransport: {
    getCapabilities: mocks.getCapabilities,
    signInWithOAuth: mocks.signInWithOAuth,
    logout: mocks.logout,
  },
}))

vi.mock('../../../shared/composables/useSiteBranding', () => ({
  useSiteBranding: () => ({
    fallbackText: 'IB',
    logoURL: ref(''),
    projectName: ref('IdeaBoard'),
  }),
}))

vi.mock('../../../shared/composables/useAppToast', () => ({
  useAppToast: () => ({ success: mocks.toastSuccess, error: mocks.toastError }),
}))

vi.mock('../../admin/pages/login/useAdminLoginFeedback', () => ({
  useAdminLoginFeedback: () => ({
    failed: vi.fn(),
    nonAdminAccount: vi.fn(),
    sessionRevokeFailed: vi.fn(),
    succeeded: vi.fn(),
  }),
}))

vi.mock('../../../shared/composables/useLogoutRedirect', () => ({
  useLogoutRedirect: () => ({ logoutAndRedirect: vi.fn() }),
}))

const ButtonStub = defineComponent({
  inheritAttrs: false,
  props: {
    disabled: { type: Boolean, default: false },
    label: { type: String, default: '' },
    type: { type: String, default: 'button' },
  },
  emits: ['click'],
  template: `
    <button v-bind="$attrs" :type="type" :disabled="disabled" @click="$emit('click', $event)">
      {{ label }}<slot />
    </button>
  `,
})

const CardStub = defineComponent({
  template:
    '<section><slot name="header" /><slot name="title" /><slot name="content" /><slot name="footer" /></section>',
})

const InputStub = defineComponent({
  inheritAttrs: false,
  props: { modelValue: { type: String, default: '' } },
  emits: ['update:modelValue'],
  template: '<input v-bind="$attrs" :value="modelValue" />',
})

function renderLoginPanel() {
  return renderWithUser(LoginAuthPanel, {
    global: {
      stubs: {
        Button: ButtonStub,
        InputText: InputStub,
        Password: InputStub,
      },
    },
  })
}

function renderAdminLogin() {
  return renderWithUser(AdminLoginPage, {
    global: {
      stubs: {
        Button: ButtonStub,
        Card: CardStub,
        IconField: { template: '<div><slot /></div>' },
        InputIcon: { template: '<span><slot /></span>' },
        InputText: InputStub,
        Password: InputStub,
        RouterLink: { template: '<a><slot /></a>' },
        SiteBrandLogo: true,
      },
    },
  })
}

function renderCallbackPage() {
  return renderWithUser(UserAuthCallbackPage, {
    global: {
      stubs: {
        Button: ButtonStub,
        ProgressSpinner: true,
      },
    },
  })
}

describe('Social OAuth entry points', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.route.path = '/user/auth/callback'
    mocks.route.query = {}
    mocks.getCapabilities.mockResolvedValue({ social: { google: false, discord: false } })
    mocks.signInWithOAuth.mockResolvedValue(undefined)
    mocks.logout.mockResolvedValue(true)
  })

  it('shows the Google button on User Login only when the capability is enabled', async () => {
    mocks.getCapabilities.mockResolvedValue({ social: { google: true, discord: false } })
    const view = renderLoginPanel()

    const googleButton = await screen.findByRole('button', { name: '使用 Google 登录' })
    expect(googleButton).toHaveTextContent('')
    expect(googleButton.querySelector('img')).toHaveAttribute('alt', '')
    expect(screen.queryByText('或')).not.toBeInTheDocument()
    await view.user.click(googleButton)

    expect(mocks.signInWithOAuth).toHaveBeenCalledWith('google')
    expect(mocks.toastSuccess).not.toHaveBeenCalled()
  })

  it('shows enabled Google and Discord buttons in one row', async () => {
    mocks.getCapabilities.mockResolvedValue({ social: { google: true, discord: true } })
    renderLoginPanel()

    const googleButton = await screen.findByRole('button', { name: '使用 Google 登录' })
    const discordButton = await screen.findByRole('button', { name: '使用 Discord 登录' })
    expect(discordButton.parentElement).toBe(googleButton.parentElement)
    expect(googleButton.parentElement?.children).toHaveLength(2)
  })

  it('hides the Google button on User Login when the capability is disabled', async () => {
    renderLoginPanel()

    await waitFor(() => expect(mocks.getCapabilities).toHaveBeenCalledOnce())
    expect(screen.queryByRole('button', { name: '使用 Google 登录' })).not.toBeInTheDocument()
    expect(screen.queryByText('或')).not.toBeInTheDocument()
  })

  it('shows an error when the Google redirect cannot start', async () => {
    mocks.getCapabilities.mockResolvedValue({ social: { google: true, discord: false } })
    mocks.signInWithOAuth.mockRejectedValue(new Error('Google 登录失败'))
    const view = renderLoginPanel()

    await view.user.click(await screen.findByRole('button', { name: '使用 Google 登录' }))

    expect(mocks.toastError).toHaveBeenCalledOnce()
    expect(mocks.toastSuccess).not.toHaveBeenCalled()
  })

  it('does not expose Google login on Admin Login', () => {
    renderAdminLogin()

    expect(screen.queryByRole('button', { name: /Google/i })).not.toBeInTheDocument()
    expect(mocks.getCapabilities).not.toHaveBeenCalled()
  })
})

describe('UserAuthCallbackPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.route.path = '/user/auth/callback'
    mocks.route.query = {}
    mocks.ensureSession.mockResolvedValue({ public_id: 'user-public-id' })
    mocks.logout.mockResolvedValue(true)
  })

  it('restores a fresh session and follows a safe internal redirect', async () => {
    mocks.route.query = { redirect: '/user/boards?view=recent#owned' }

    renderCallbackPage()

    await waitFor(() => {
      expect(mocks.ensureSession).toHaveBeenCalledWith({ fresh: true })
      expect(mocks.routerReplace).toHaveBeenCalledWith('/user/boards?view=recent#owned')
    })
    expect(mocks.logout).not.toHaveBeenCalled()
    expect(mocks.clearAuth).not.toHaveBeenCalled()
  })

  it('rejects an external redirect and falls back to User Boards', async () => {
    mocks.route.query = { redirect: 'https://attacker.example/steal-session' }

    renderCallbackPage()

    await waitFor(() => expect(mocks.routerReplace).toHaveBeenCalledWith('/user/boards'))
    expect(mocks.routerReplace).not.toHaveBeenCalledWith(
      expect.stringContaining('attacker.example'),
    )
  })

  it('logs out, clears local auth, removes callback details, and shows only a generic error', async () => {
    mocks.route.query = {
      error: 'access_denied:provider-raw-detail',
      redirect: '/user/boards',
    }

    renderCallbackPage()

    expect(await screen.findByText('OAuth 登录失败，请重新尝试。')).toBeVisible()
    expect(mocks.ensureSession).not.toHaveBeenCalled()
    expect(mocks.logout).toHaveBeenCalledOnce()
    expect(mocks.clearAuth).toHaveBeenCalledOnce()
    expect(mocks.routerReplace).toHaveBeenCalledWith({
      path: '/user/auth/callback',
      query: {},
    })
    expect(screen.queryByText(/provider-raw-detail/)).not.toBeInTheDocument()
  })
})
