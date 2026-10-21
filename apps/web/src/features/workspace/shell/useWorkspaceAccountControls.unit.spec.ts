import { defineComponent, h, ref } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render, waitFor } from '../../../test/render'
import type { User } from '../../../api/sessionUserType'
import { ADMIN_ROOT_PATH } from '../../admin/routePaths'
import { HOME_PATH } from '../routePaths'
import {
  createAccountOverlay,
  provideAccountOverlay,
  type AccountOverlay,
  type CreditWalletState,
} from '../../account'
import { authTransport, createAuthOverlay, provideAuthOverlay, type AuthOverlay } from '../../auth'
import { useWorkspaceAccountControls } from './useWorkspaceAccountControls'
import { makeSessionUser } from '../../../test/factories'

const currentUser = ref<User | null>(null)
const sessionEmail = ref<string | null>(null)
const walletState = ref<CreditWalletState>({
  status: 'ready',
  availableCredits: 1280,
  billingAccountId: 'ba_1',
  isShared: false,
})

vi.mock('../../auth/session/transport', () => ({
  authTransport: { getSessionUser: vi.fn() },
}))

vi.mock('../../auth/session/userSession', () => ({
  useUserStore: () => ({
    get user() {
      return currentUser.value
    },
    get email() {
      return sessionEmail.value
    },
    setEmail(email: string) {
      sessionEmail.value = email
    },
  }),
}))

vi.mock('../../account', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../account')>()
  return {
    ...actual,
    useCreditWallet: () => ({ state: walletState }),
  }
})

function renderAccountControls(options: Parameters<typeof useWorkspaceAccountControls>[0]): {
  controls: ReturnType<typeof useWorkspaceAccountControls>
  overlay: AccountOverlay
  authOverlay: AuthOverlay
} {
  let controls: ReturnType<typeof useWorkspaceAccountControls> | undefined
  let overlay: AccountOverlay | undefined
  let authOverlay: AuthOverlay | undefined
  // The consumer must be a descendant because Vue does not expose a component's own provides to it.
  const Consumer = defineComponent({
    setup() {
      controls = useWorkspaceAccountControls(options)
      return () => h('div')
    },
  })
  const Harness = defineComponent({
    setup() {
      overlay = createAccountOverlay({
        currentUser,
        reportError: vi.fn(),
        navigation: {
          replace: vi.fn().mockResolvedValue(undefined),
          resolve: (location) => ({ fullPath: location.path }),
        },
        route: { fullPath: '/user/boards', hash: '', path: '/user/boards', query: {} },
      })
      provideAccountOverlay(overlay)
      authOverlay = createAuthOverlay()
      provideAuthOverlay(authOverlay)
      return () => h(Consumer)
    },
  })
  render(Harness)
  if (!controls || !overlay || !authOverlay) {
    throw new Error('Account controls harness failed to initialize')
  }
  return { controls, overlay, authOverlay }
}

describe('useWorkspaceAccountControls', () => {
  beforeEach(() => {
    window.history.replaceState({}, '', '/')
    currentUser.value = makeSessionUser({
      public_id: '00000000-0000-0000-0000-000000000001',
      display_name: 'demo-user',
      avatar: 'https://example.test/avatar.png',
    })
    sessionEmail.value = null
    vi.mocked(authTransport.getSessionUser).mockReset().mockResolvedValue(null)
    walletState.value = {
      status: 'ready',
      availableCredits: 1280,
      billingAccountId: 'ba_1',
      isShared: false,
    }
  })

  it('exposes typed balance, avatar and session email without a credits menu item', () => {
    // Menu email comes from the Better Auth session store (issue #913),
    // never from the business principal projection.
    sessionEmail.value = 'demo@example.test'
    const { controls } = renderAccountControls({
      navigation: { navigate: vi.fn(), logout: vi.fn() },
    })

    expect(controls.viewModel.avatarUrl.value).toBe('https://example.test/avatar.png')
    expect(controls.viewModel.userEmail.value).toBe('demo@example.test')
    expect(controls.viewModel.balance.value).toMatchObject({
      availableIdeas: 1280,
      displayValue: '1,280',
      ariaLabel: 'Ideas balance: 1,280 Ideas',
      status: 'ready',
    })
    expect(controls.viewModel.menuItems.value.map((item) => item.key)).toEqual([
      'profile',
      'home',
      'logout',
    ])
  })

  it('sizes the public avatar route to the next variant up (32px chrome at dpr 1 → 32)', () => {
    currentUser.value = makeSessionUser({
      public_id: '00000000-0000-0000-0000-000000000001',
      display_name: 'demo-user',
      avatar: '/api/v1/public/users/AVTR01/avatar',
    })
    const { controls } = renderAccountControls({
      navigation: { navigate: vi.fn(), logout: vi.fn() },
    })

    expect(controls.viewModel.avatarUrl.value).toBe('/api/v1/public/users/AVTR01/avatar?size=32')
  })

  it('falls back to the session store email when the user projection omits email', () => {
    currentUser.value = makeSessionUser({
      public_id: '00000000-0000-0000-0000-000000000001',
      display_name: 'demo-user',
    })
    sessionEmail.value = 'session@example.test'
    const { controls } = renderAccountControls({
      navigation: { navigate: vi.fn(), logout: vi.fn() },
    })

    expect(controls.viewModel.userEmail.value).toBe('session@example.test')
  })

  it('hydrates the email from the auth session when the user projection omits it', async () => {
    currentUser.value = makeSessionUser({
      public_id: '00000000-0000-0000-0000-000000000001',
      display_name: 'demo-user',
    })
    vi.mocked(authTransport.getSessionUser).mockResolvedValue({ email: 'alice@example.test' })
    const { controls } = renderAccountControls({
      navigation: { navigate: vi.fn(), logout: vi.fn() },
    })

    await waitFor(() => {
      expect(controls.viewModel.userEmail.value).toBe('alice@example.test')
    })
    expect(authTransport.getSessionUser).toHaveBeenCalledOnce()
  })

  it('exposes an anonymous balance state and guest menu items when user is not logged in', () => {
    currentUser.value = null
    const { controls } = renderAccountControls({
      navigation: { navigate: vi.fn(), logout: vi.fn() },
    })

    expect(controls.viewModel.balance.value).toEqual({
      status: 'anonymous',
      displayValue: '',
      ariaLabel: 'Ideas',
    })
    expect(controls.viewModel.menuItems.value.map((item) => item.key)).toEqual([
      'login',
      'register',
      'home',
    ])
  })

  it('opens login and registration through the installed auth overlay', () => {
    const navigate = vi.fn()
    const { controls, authOverlay } = renderAccountControls({
      navigation: { navigate, logout: vi.fn() },
    })

    controls.commands.openLogin()
    expect(authOverlay.viewModel.mode.value).toBe('login')

    controls.commands.openRegister()
    expect(authOverlay.viewModel.mode.value).toBe('register')
    expect(navigate).not.toHaveBeenCalled()
  })

  it('keeps balance loading state typed and opens the billing tab from the balance control', () => {
    walletState.value = { status: 'loading' }
    const { controls, overlay } = renderAccountControls({
      navigation: { navigate: vi.fn(), logout: vi.fn() },
    })

    expect(controls.viewModel.balance.value).toMatchObject({
      status: 'loading',
      displayValue: 'Loading…',
      ariaLabel: 'Ideas balance is loading',
    })

    controls.viewModel.balance.value.command?.()
    expect(overlay.viewModel.initialTab.value).toBe('billing')
    expect(overlay.viewModel.isVisible.value).toBe(true)
  })

  it('exposes a recoverable error instead of leaving a failed balance busy', () => {
    walletState.value = { status: 'error', error: new Error('wallet unavailable') }
    const { controls, overlay } = renderAccountControls({
      navigation: { navigate: vi.fn(), logout: vi.fn() },
    })

    expect(controls.viewModel.balance.value).toMatchObject({
      status: 'error',
      displayValue: '--',
      ariaLabel: 'Could not load your Ideas balance. Open your account.',
      errorMessage: 'Billing information is unavailable right now.',
    })

    controls.viewModel.balance.value.command?.()
    expect(overlay.viewModel.initialTab.value).toBe('billing')
    expect(overlay.viewModel.isVisible.value).toBe(true)
  })

  it('routes account controls through the supplied navigation adapter and exposes admin only to admins', async () => {
    currentUser.value = makeSessionUser({
      public_id: '00000000-0000-0000-0000-000000000001',
      display_name: 'demo-user',
      avatar: 'https://example.test/avatar.png',
      role: 'admin',
    })
    const navigate = vi.fn()
    const logout = vi.fn()
    const { controls, overlay } = renderAccountControls({
      navigation: { navigate, logout },
      shouldShowHome: false,
    })
    const items = controls.viewModel.menuItems.value

    expect(items.map((item) => item.key)).toEqual(['profile', 'admin', 'logout'])

    items.find((item) => item.key === 'profile')?.command()
    items.find((item) => item.key === 'admin')?.command()
    items.find((item) => item.key === 'logout')?.command()
    controls.commands.goHome()

    expect(overlay.viewModel.initialTab.value).toBe('profile')
    expect(overlay.viewModel.isVisible.value).toBe(true)
    expect(navigate).toHaveBeenCalledWith(ADMIN_ROOT_PATH)
    expect(navigate).toHaveBeenCalledWith(HOME_PATH)
    expect(logout).toHaveBeenCalledOnce()
  })
})
