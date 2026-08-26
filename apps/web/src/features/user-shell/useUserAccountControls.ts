import { computed } from 'vue'
import { useUserStore } from '../auth/session/authSession'
import { canAccessAdmin } from '../../shared/auth/adminAccess'
import type {
  UserAccountMenuItem,
  UserIdeaBalanceControl,
} from '../../shared/components/userAccountMenu.types'
import { ADMIN_ROOT_PATH, HOME_PATH } from '../../shared/constants/routePaths'
import { i18n } from '../../shared/i18n'
import { formatLocaleNumber } from '../../shared/i18n/formatters'
import { userFacingErrorCopy } from '../../shared/errors/errorCatalog'
import { formatIdeaAmount } from '../../shared/i18n/userIdeaCurrency'
import {
  defaultAccountManagementTab,
  type AccountManagementTab,
} from '../account-management/accountTabRegistry'
import { useAccountManagementOverlay } from '../account-management/accountManagementOverlay'
import { useUserCreditWalletSummary } from '../account-management/useUserCreditWalletSummary'
import { useAuthOverlay } from '../auth/authOverlay'

export type UserShellNavigationAdapter = {
  navigate: (path: string) => Promise<unknown> | unknown
  logout: () => Promise<unknown> | unknown
}

type UseUserAccountControlsOptions = {
  navigation: UserShellNavigationAdapter
  showHome?: boolean
}

function formatIdeaValue(value: number): string {
  return formatLocaleNumber(value)
}

export function useUserAccountControls(options: UseUserAccountControlsOptions) {
  const userStore = useUserStore()
  const wallet = useUserCreditWalletSummary(() => Boolean(userStore.user))
  const accountManagement = useAccountManagementOverlay()

  const authOverlay = useAuthOverlay()

  function openAccount(tab: AccountManagementTab = defaultAccountManagementTab) {
    accountManagement.commands.open(tab)
  }

  function openLogin() {
    authOverlay.commands.open('login')
  }

  function openRegister() {
    authOverlay.commands.open('register')
  }

  function goAdmin() {
    return options.navigation.navigate(ADMIN_ROOT_PATH)
  }

  function goHome() {
    return options.navigation.navigate(HOME_PATH)
  }

  function logout() {
    return options.navigation.logout()
  }

  const balance = computed<UserIdeaBalanceControl>(() => {
    // Track locale for reactivity of number formatting
    void i18n.global.locale.value
    if (!userStore.user) {
      return {
        status: 'anonymous',
        displayValue: '',
        ariaLabel: i18n.global.t('auth.credits.badgeAriaLabel'),
      }
    }
    const command = () => openAccount('billing')
    const state = wallet.state.value
    switch (state.status) {
      case 'loading':
        return {
          status: 'loading',
          displayValue: i18n.global.t('common.actions.loading'),
          ariaLabel: i18n.global.t('auth.credits.loadingAriaLabel'),
          command,
        }
      case 'error':
        return {
          status: 'error',
          displayValue: '--',
          ariaLabel: i18n.global.t('auth.credits.errorAriaLabel'),
          errorMessage: userFacingErrorCopy(
            state.error,
            i18n.global.t('common.errors.billingInfoUnavailable'),
          ),
          command,
        }
      case 'ready': {
        const availableIdeas = state.availableCredits
        return {
          status: 'ready',
          availableIdeas,
          displayValue: formatIdeaValue(availableIdeas),
          ariaLabel: i18n.global.t('auth.credits.balanceAriaLabel', {
            amount: formatIdeaAmount(availableIdeas),
          }),
          command,
        }
      }
    }
    throw new Error('Unknown wallet summary state')
  })

  const menuItems = computed<UserAccountMenuItem[]>(() => {
    if (!userStore.user) {
      const guestItems: UserAccountMenuItem[] = [
        { key: 'login', label: i18n.global.t('auth.signIn'), command: () => openLogin() },
        { key: 'register', label: i18n.global.t('auth.signUpFree'), command: () => openRegister() },
      ]
      if (options.showHome ?? true) {
        guestItems.push({
          key: 'home',
          label: i18n.global.t('auth.menu.home'),
          command: () => void goHome(),
        })
      }
      return guestItems
    }

    const items: UserAccountMenuItem[] = [
      { key: 'profile', label: i18n.global.t('auth.menu.account'), command: () => openAccount() },
    ]
    if (canAccessAdmin(userStore.user)) {
      items.push({
        key: 'admin',
        label: i18n.global.t('auth.menu.console'),
        command: () => void goAdmin(),
      })
    }
    if (options.showHome ?? true) {
      items.push({
        key: 'home',
        label: i18n.global.t('auth.menu.home'),
        command: () => void goHome(),
      })
    }
    items.push({
      key: 'logout',
      label: i18n.global.t('auth.signOut'),
      tone: 'danger',
      command: () => void logout(),
    })
    return items
  })

  return {
    viewModel: {
      avatarUrl: computed(() => userStore.user?.avatar || undefined),
      balance,
      menuItems,
    },
    commands: { openAccount, openLogin, openRegister, goAdmin, goHome, logout },
  }
}
