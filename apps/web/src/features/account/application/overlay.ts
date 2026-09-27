import { computed, inject, provide, readonly, ref, watch, type InjectionKey, type Ref } from 'vue'
import type { LocationQuery, LocationQueryRaw } from 'vue-router'
import {
  ADMIN_ROOT_PATH,
  USER_LOGIN_PATH,
  USER_REGISTER_PATH,
  USER_BOARDS_PATH,
  isPathUnderRoute,
} from '../../../shared/constants/routePaths'
import { stringParam } from '../../../shared/router/queryParams'
import {
  defaultAccountTab,
  parsePaymentReturnQuery,
  resolveAccountTabKey,
  stripPaymentReturnQuery,
  type AccountTab,
  type PaymentReturnContext,
} from '../domain'

export type AccountOverlayRoute = {
  fullPath: string
  hash: string
  path: string
  query: LocationQuery
}

export type AccountOverlayLocation = {
  hash?: string
  path: string
  query?: LocationQueryRaw
}

export type AccountOverlayOptions = {
  currentUser: Readonly<Ref<unknown>>
  reportError(error: Error): void
  navigation: {
    replace(location: AccountOverlayLocation): Promise<unknown>
    resolve(location: AccountOverlayLocation): { fullPath: string }
  }
  route: AccountOverlayRoute
}

export function createAccountOverlay(options: AccountOverlayOptions) {
  const visible = ref(false)
  const initialTab = ref<AccountTab>(defaultAccountTab)
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
    tab: AccountTab = defaultAccountTab,
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

      const requestedTab =
        isAllowed && options.route.path === USER_BOARDS_PATH
          ? stringParam(options.route.query.accountTab)
          : undefined
      if (requestedTab) {
        open(resolveAccountTabKey(requestedTab))
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

export type AccountOverlay = ReturnType<typeof createAccountOverlay>

const accountOverlayKey: InjectionKey<AccountOverlay> = Symbol('account-overlay')

export function provideAccountOverlay(overlay: AccountOverlay) {
  provide(accountOverlayKey, overlay)
}

export function useAccountOverlay(): AccountOverlay {
  const overlay = inject(accountOverlayKey)
  if (!overlay) {
    throw new Error('Account overlay is not installed by User Shell')
  }
  return overlay
}
