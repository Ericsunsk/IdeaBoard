import { computed, inject, provide, readonly, ref, watch, type InjectionKey, type Ref } from 'vue'
import type { LocationQuery, LocationQueryRaw } from 'vue-router'
import {
  ADMIN_ROOT_PATH,
  USER_LOGIN_PATH,
  USER_REGISTER_PATH,
  isPathUnderRoute,
} from '../../shared/constants/routePaths'
import { defaultAccountManagementTab, type AccountManagementTab } from './accountTabRegistry'
import {
  parsePaymentReturnQuery,
  stripPaymentReturnQuery,
  type PaymentReturnContext,
} from './paymentReturnRoute'

type AccountManagementOverlayRoute = {
  fullPath: string
  hash: string
  path: string
  query: LocationQuery
}

type AccountManagementOverlayLocation = {
  hash?: string
  path: string
  query?: LocationQueryRaw
}

type AccountManagementOverlayOptions = {
  currentUser: Readonly<Ref<unknown>>
  reportError(error: Error): void
  navigation: {
    replace(location: AccountManagementOverlayLocation): Promise<unknown>
    resolve(location: AccountManagementOverlayLocation): { fullPath: string }
  }
  route: AccountManagementOverlayRoute
}

export function createAccountManagementOverlay(options: AccountManagementOverlayOptions) {
  const visible = ref(false)
  const initialTab = ref<AccountManagementTab>(defaultAccountManagementTab)
  const paymentReturn = ref<PaymentReturnContext | null>(null)
  let previousFullPath: string | undefined
  let paymentReturnCleanupSequence = 0
  let pendingPaymentReturnCleanup: {
    expectedFullPath: string
    sequence: number
    sourceFullPath: string
  } | null = null
  const allowed = computed(
    () =>
      Boolean(options.currentUser.value) &&
      !isPathUnderRoute(options.route.path, ADMIN_ROOT_PATH) &&
      !isPathUnderRoute(options.route.path, USER_LOGIN_PATH) &&
      !isPathUnderRoute(options.route.path, USER_REGISTER_PATH),
  )

  function close() {
    visible.value = false
    paymentReturn.value = null
  }

  function open(
    tab: AccountManagementTab = defaultAccountManagementTab,
    returnContext: PaymentReturnContext | null = null,
  ) {
    if (!allowed.value) return
    initialTab.value = tab
    paymentReturn.value = returnContext
    visible.value = true
  }

  function setVisible(nextVisible: boolean) {
    if (!nextVisible) {
      close()
      return
    }
    if (allowed.value) visible.value = true
  }

  watch(
    () => [options.route.fullPath, allowed.value] as const,
    ([fullPath, isAllowed]) => {
      const routeChanged = previousFullPath !== undefined && previousFullPath !== fullPath
      previousFullPath = fullPath

      if (pendingPaymentReturnCleanup) {
        const expectedCleanup = pendingPaymentReturnCleanup
        if (fullPath === expectedCleanup.expectedFullPath) {
          pendingPaymentReturnCleanup = null
          return
        }
        if (fullPath !== expectedCleanup.sourceFullPath) pendingPaymentReturnCleanup = null
      }

      const returnContext = parsePaymentReturnQuery(options.route.query)
      if (returnContext && isAllowed) {
        open('recharge', returnContext)
        const cleanupLocation = {
          hash: options.route.hash,
          path: options.route.path,
          query: stripPaymentReturnQuery(options.route.query),
        }
        const expectedCleanup = {
          expectedFullPath: options.navigation.resolve(cleanupLocation).fullPath,
          sequence: ++paymentReturnCleanupSequence,
          sourceFullPath: fullPath,
        }
        pendingPaymentReturnCleanup = expectedCleanup
        void Promise.resolve().then(async () => {
          if (
            pendingPaymentReturnCleanup?.sequence !== expectedCleanup.sequence ||
            options.route.fullPath !== expectedCleanup.sourceFullPath ||
            !allowed.value
          ) {
            if (pendingPaymentReturnCleanup?.sequence === expectedCleanup.sequence) {
              pendingPaymentReturnCleanup = null
            }
            return
          }
          try {
            await options.navigation.replace(cleanupLocation)
          } catch (error) {
            if (pendingPaymentReturnCleanup?.sequence === expectedCleanup.sequence) {
              pendingPaymentReturnCleanup = null
            }
            options.reportError(
              new Error('Failed to clean the Account Management payment-return URL', {
                cause: error,
              }),
            )
          }
        })
        return
      }

      if (!isAllowed || routeChanged) close()
    },
    { immediate: true },
  )

  return {
    viewModel: {
      allowed,
      initialTab: readonly(initialTab),
      paymentReturn: readonly(paymentReturn),
      visible: readonly(visible),
    },
    commands: { close, open, setVisible },
  }
}

export type AccountManagementOverlay = ReturnType<typeof createAccountManagementOverlay>

const accountManagementOverlayKey: InjectionKey<AccountManagementOverlay> = Symbol(
  'account-management-overlay',
)

export function provideAccountManagementOverlay(overlay: AccountManagementOverlay) {
  provide(accountManagementOverlayKey, overlay)
}

export function useAccountManagementOverlay(): AccountManagementOverlay {
  const overlay = inject(accountManagementOverlayKey)
  if (!overlay) {
    throw new Error('Account Management overlay is not installed by User Shell')
  }
  return overlay
}
