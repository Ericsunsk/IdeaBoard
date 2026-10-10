import { computed, onMounted } from 'vue'
import { authTransport, useAuthOverlay, useUserStore } from '../../auth'
import { canAccessAdmin } from '../../../core/auth/adminAccess'
import type {
  UserAccountMenuItem,
  UserIdeaBalanceControl,
} from '../../account/ui/chrome/userAccountMenu.types'
import { ADMIN_ROOT_PATH } from '../../admin/routePaths'
import { HOME_PATH } from '../routePaths'
import { i18n } from '../../../core/i18n'
import { formatLocaleNumber } from '../../../core/i18n/formatters'
import { userFacingErrorCopy } from '../../../core/errors/errorCatalog'
import { formatIdeaAmount } from '../../../core/i18n/userIdeaCurrency'
import {
  defaultAccountTab,
  useAccountOverlay,
  useCreditWallet,
  type AccountTab,
} from '../../account'

export type WorkspaceNavigationAdapter = {
  navigate: (path: string) => Promise<unknown> | unknown
  logout: () => Promise<unknown> | unknown
}

type UseWorkspaceAccountControlsOptions = {
  navigation: WorkspaceNavigationAdapter
  showHome?: boolean
}

function formatIdeaValue(value: number): string {
  return formatLocaleNumber(value)
}

export function useWorkspaceAccountControls(options: UseWorkspaceAccountControlsOptions) {
  const userStore = useUserStore()
  onMounted(() => {
    const currentUser = userStore.user
    // Identity email comes only from the Better Auth chain (issue #913):
    // the business principal never carries an email, so hydrate the
    // session-store email instead of reading it off the user projection.
    if (!currentUser || userStore.email) return

    const userId = currentUser.public_id
    void authTransport
      .getSessionUser()
      .then((sessionUser) => {
        if (!sessionUser?.email) return
        if (userStore.user?.public_id !== userId || userStore.email) {
          return
        }
        userStore.setEmail(sessionUser.email)
      })
      .catch(() => {
        // Session email hydration must not block workspace controls.
      })
  })
  // Validated session only (not the best-effort restored snapshot): this
  // header mounts on public pages too, and an anonymous/stale 401 reloads
  // the whole page to login via the shared auth-failure handler.
  const wallet = useCreditWallet(() => Boolean(userStore.user) && userStore.sessionValidatedAt > 0)
  const accountOverlay = useAccountOverlay()

  const authOverlay = useAuthOverlay()

  function openAccount(tab: AccountTab = defaultAccountTab) {
    accountOverlay.commands.open(tab)
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
      // Menu email display stays on the Better Auth chain (issue #913):
      // the business principal has no email field to read.
      userEmail: computed(() => userStore.email || undefined),
      balance,
      menuItems,
    },
    commands: { openAccount, openLogin, openRegister, goAdmin, goHome, logout },
  }
}
