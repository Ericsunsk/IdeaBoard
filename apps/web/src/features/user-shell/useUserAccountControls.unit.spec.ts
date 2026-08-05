import { ref } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { User } from '../../shared/api/userSession'
import { ADMIN_ROOT_PATH, USER_HOME_PATH } from '../../shared/constants/routePaths'
import type { UserCreditWalletSummaryState } from '../account-management/useUserCreditWalletSummary'
import { useUserAccountControls } from './useUserAccountControls'

const mocks = vi.hoisted(() => ({
  openAccountManagementDialog: vi.fn(),
}))

const currentUser = ref<User | null>(null)
const walletState = ref<UserCreditWalletSummaryState>({
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

vi.mock('../account-management/useUserCreditWalletSummary', () => ({
  useUserCreditWalletSummary: () => ({ state: walletState }),
}))

vi.mock('../account-management/useAccountManagementDialogController', () => ({
  openAccountManagementDialog: mocks.openAccountManagementDialog,
}))

function makeUser(role: 'admin' | 'user' = 'user'): User {
  return {
    id: 1,
    uid: 'USR001',
    username: 'demo-user',
    avatar: 'https://example.test/avatar.png',
    role,
    status: 1,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
  }
}

describe('useUserAccountControls', () => {
  beforeEach(() => {
    currentUser.value = makeUser()
    walletState.value = { status: 'ready', availableCredits: 1280 }
    mocks.openAccountManagementDialog.mockReset()
  })

  it('exposes typed balance, avatar and regular-user controls without a credits menu item', () => {
    const controls = useUserAccountControls({
      navigation: { navigate: vi.fn(), logout: vi.fn() },
    })

    expect(controls.viewModel.avatarUrl.value).toBe('https://example.test/avatar.png')
    expect(controls.viewModel.balance.value).toMatchObject({
      availableIdeas: 1280,
      displayValue: '1,280',
      ariaLabel: 'Idea余额 1,280 Ideas',
      status: 'ready',
    })
    expect(controls.viewModel.menuItems.value.map((item) => item.key)).toEqual([
      'profile',
      'home',
      'logout',
    ])
  })

  it('keeps balance loading state typed and opens the billing tab from the balance control', () => {
    walletState.value = { status: 'loading' }
    const controls = useUserAccountControls({
      navigation: { navigate: vi.fn(), logout: vi.fn() },
    })

    expect(controls.viewModel.balance.value).toMatchObject({
      status: 'loading',
      displayValue: '加载中',
      ariaLabel: 'Idea余额 加载中',
    })

    controls.viewModel.balance.value.command()
    expect(mocks.openAccountManagementDialog).toHaveBeenCalledWith('billing')
  })

  it('exposes a recoverable error instead of leaving a failed balance busy', () => {
    walletState.value = { status: 'error', error: new Error('wallet unavailable') }
    const controls = useUserAccountControls({
      navigation: { navigate: vi.fn(), logout: vi.fn() },
    })

    expect(controls.viewModel.balance.value).toMatchObject({
      status: 'error',
      displayValue: '--',
      ariaLabel: 'Idea余额加载失败，点击查看账户',
      errorMessage: 'wallet unavailable',
    })

    controls.viewModel.balance.value.command()
    expect(mocks.openAccountManagementDialog).toHaveBeenCalledWith('billing')
  })

  it('routes account controls through the supplied navigation adapter and exposes admin only to admins', async () => {
    currentUser.value = makeUser('admin')
    const navigate = vi.fn()
    const logout = vi.fn()
    const controls = useUserAccountControls({ navigation: { navigate, logout }, showHome: false })
    const items = controls.viewModel.menuItems.value

    expect(items.map((item) => item.key)).toEqual(['profile', 'admin', 'logout'])

    items.find((item) => item.key === 'profile')?.command()
    items.find((item) => item.key === 'admin')?.command()
    items.find((item) => item.key === 'logout')?.command()
    controls.commands.goHome()

    expect(mocks.openAccountManagementDialog).toHaveBeenCalledWith('profile')
    expect(navigate).toHaveBeenCalledWith(ADMIN_ROOT_PATH)
    expect(navigate).toHaveBeenCalledWith(USER_HOME_PATH)
    expect(logout).toHaveBeenCalledOnce()
  })
})
