import { computed } from 'vue'
import { useUserStore } from '../../app/stores/user'
import { canAccessAdmin } from '../../shared/auth/adminAccess'
import type {
  UserAccountMenuItem,
  UserIdeaBalanceControl,
} from '../../shared/components/userAccountMenu.types'
import { ADMIN_ROOT_PATH, USER_HOME_PATH } from '../../shared/constants/routePaths'
import { formatIdeaAmount } from '../../shared/utils/ideaCurrency'
import {
  defaultAccountManagementTab,
  type AccountManagementTab,
} from '../account-management/accountTabRegistry'
import { openAccountManagementDialog } from '../account-management/useAccountManagementDialogController'
import { useUserCreditWalletSummary } from '../account-management/useUserCreditWalletSummary'

export type UserShellNavigationAdapter = {
  navigate: (path: string) => Promise<unknown> | unknown
  logout: () => Promise<unknown> | unknown
}

type UseUserAccountControlsOptions = {
  navigation: UserShellNavigationAdapter
  showHome?: boolean
}

function formatIdeaValue(value: number): string {
  return new Intl.NumberFormat('en-US').format(value)
}

export function useUserAccountControls(options: UseUserAccountControlsOptions) {
  const userStore = useUserStore()
  const wallet = useUserCreditWalletSummary(() => Boolean(userStore.user))

  function openAccount(tab: AccountManagementTab = defaultAccountManagementTab) {
    openAccountManagementDialog(tab)
  }

  function goAdmin() {
    return options.navigation.navigate(ADMIN_ROOT_PATH)
  }

  function goHome() {
    return options.navigation.navigate(USER_HOME_PATH)
  }

  function logout() {
    return options.navigation.logout()
  }

  const balance = computed<UserIdeaBalanceControl>(() => {
    const command = () => openAccount('billing')
    const state = wallet.state.value
    switch (state.status) {
      case 'loading':
        return {
          status: 'loading',
          displayValue: '加载中',
          ariaLabel: 'Idea余额 加载中',
          command,
        }
      case 'error':
        return {
          status: 'error',
          displayValue: '--',
          ariaLabel: 'Idea余额加载失败，点击查看账户',
          errorMessage: state.error.message || 'Idea余额加载失败',
          command,
        }
      case 'ready': {
        const availableIdeas = state.availableCredits
        return {
          status: 'ready',
          availableIdeas,
          displayValue: formatIdeaValue(availableIdeas),
          ariaLabel: `Idea余额 ${formatIdeaAmount(availableIdeas)}`,
          command,
        }
      }
    }
    throw new Error('Unknown wallet summary state')
  })

  const menuItems = computed<UserAccountMenuItem[]>(() => {
    const items: UserAccountMenuItem[] = [
      { key: 'profile', label: '账户管理', command: () => openAccount() },
    ]
    if (canAccessAdmin(userStore.user)) {
      items.push({ key: 'admin', label: '管理后台', command: () => void goAdmin() })
    }
    if (options.showHome ?? true) {
      items.push({ key: 'home', label: '返回首页', command: () => void goHome() })
    }
    items.push({ key: 'logout', label: '退出登录', tone: 'danger', command: () => void logout() })
    return items
  })

  return {
    viewModel: {
      avatarUrl: computed(() => userStore.user?.avatar || undefined),
      balance,
      menuItems,
    },
    commands: { openAccount, goAdmin, goHome, logout },
  }
}
