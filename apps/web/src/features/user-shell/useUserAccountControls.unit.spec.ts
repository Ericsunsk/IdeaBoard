import { defineComponent, h, ref } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render } from '../../test/render'
import type { User } from '../../shared/api/userSession'
import { ADMIN_ROOT_PATH, HOME_PATH } from '../../shared/constants/routePaths'
import {
  createAccountOverlay,
  provideAccountOverlay,
  type AccountOverlay,
  type CreditWalletState,
} from '../account'
import { createAuthOverlay, provideAuthOverlay, type AuthOverlay } from '../auth/authOverlay'
import { useUserAccountControls } from './useUserAccountControls'

const currentUser = ref<User | null>(null)
const walletState = ref<CreditWalletState>({
  status: 'ready',
  availableCredits: 1280,
})

vi.mock('../auth/session/authSession', () => ({
  useUserStore: () => ({
    get user() {
      return currentUser.value
    },
  }),
}))

vi.mock('../account', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../account')>()
  return {
    ...actual,
    useCreditWallet: () => ({ state: walletState }),
  }
})

function makeUser(role: 'admin' | 'user' = 'user'): User {
  return {
    public_id: '00000000-0000-0000-0000-000000000001',
    display_name: 'demo-user',
    storage_namespace: 'USR001',
    avatar: 'https://example.test/avatar.png',
    role,
    status: 1,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
  }
}

function renderAccountControls(options: Parameters<typeof useUserAccountControls>[0]): {
  controls: ReturnType<typeof useUserAccountControls>
  overlay: AccountOverlay
  authOverlay: AuthOverlay
} {
  let controls: ReturnType<typeof useUserAccountControls> | undefined
  let overlay: AccountOverlay | undefined
  let authOverlay: AuthOverlay | undefined
  // The consumer must be a descendant because Vue does not expose a component's own provides to it.
  // eslint-disable-next-line vue/one-component-per-file
  const Consumer = defineComponent({
    setup() {
      controls = useUserAccountControls(options)
      return () => h('div')
    },
  })
  // eslint-disable-next-line vue/one-component-per-file
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

describe('useUserAccountControls', () => {
  beforeEach(() => {
    window.history.replaceState({}, '', '/')
    currentUser.value = makeUser()
    walletState.value = { status: 'ready', availableCredits: 1280 }
  })

  it('exposes typed balance, avatar and regular-user controls without a credits menu item', () => {
    const { controls } = renderAccountControls({
      navigation: { navigate: vi.fn(), logout: vi.fn() },
    })

    expect(controls.viewModel.avatarUrl.value).toBe('https://example.test/avatar.png')
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
    expect(overlay.viewModel.visible.value).toBe(true)
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
    expect(overlay.viewModel.visible.value).toBe(true)
  })

  it('routes account controls through the supplied navigation adapter and exposes admin only to admins', async () => {
    currentUser.value = makeUser('admin')
    const navigate = vi.fn()
    const logout = vi.fn()
    const { controls, overlay } = renderAccountControls({
      navigation: { navigate, logout },
      showHome: false,
    })
    const items = controls.viewModel.menuItems.value

    expect(items.map((item) => item.key)).toEqual(['profile', 'admin', 'logout'])

    items.find((item) => item.key === 'profile')?.command()
    items.find((item) => item.key === 'admin')?.command()
    items.find((item) => item.key === 'logout')?.command()
    controls.commands.goHome()

    expect(overlay.viewModel.initialTab.value).toBe('profile')
    expect(overlay.viewModel.visible.value).toBe(true)
    expect(navigate).toHaveBeenCalledWith(ADMIN_ROOT_PATH)
    expect(navigate).toHaveBeenCalledWith(HOME_PATH)
    expect(logout).toHaveBeenCalledOnce()
  })
})
