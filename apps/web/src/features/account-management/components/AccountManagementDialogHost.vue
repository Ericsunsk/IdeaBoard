<template>
  <AccountManagementDialog
    v-if="isHostAllowed"
    :visible="visible"
    :initial-tab="initialTab"
    :payment-return="paymentReturn"
    @update:visible="setVisible"
  />
</template>

<script setup lang="ts">
import { computed, watch } from 'vue'
import { useRoute, useRouter, type LocationQueryRaw } from 'vue-router'
import { useUserStore } from '../../../app/stores/user'
import {
  ADMIN_ROOT_PATH,
  USER_LOGIN_PATH,
  USER_REGISTER_PATH,
  isPathUnderRoute,
} from '../../../shared/constants/routePaths'
import AccountManagementDialog from './AccountManagementDialog.vue'
import { useAccountManagementDialogController } from '../useAccountManagementDialogController'
import { parsePaymentReturnQuery, stripPaymentReturnQuery } from '../paymentReturnRoute'

const route = useRoute()
const router = useRouter()
const userStore = useUserStore()
const { visible, initialTab, paymentReturn, open, close, setVisible } =
  useAccountManagementDialogController()

const isAuthRoute = computed(
  () =>
    isPathUnderRoute(route.path, USER_LOGIN_PATH) ||
    isPathUnderRoute(route.path, USER_REGISTER_PATH),
)
const isHostAllowed = computed(
  () =>
    Boolean(userStore.user) && !isPathUnderRoute(route.path, ADMIN_ROOT_PATH) && !isAuthRoute.value,
)

let previousFullPath: string | undefined
let pendingPaymentReturnCleanup: { expectedFullPath: string; publicToken: string } | null = null

watch(
  () => [route.fullPath, isHostAllowed.value] as const,
  async ([fullPath]) => {
    const routeChanged = previousFullPath !== undefined && previousFullPath !== fullPath
    previousFullPath = fullPath

    if (pendingPaymentReturnCleanup) {
      const expectedCleanup = pendingPaymentReturnCleanup
      pendingPaymentReturnCleanup = null
      if (fullPath === expectedCleanup.expectedFullPath) {
        return
      }
    }

    const returnContext = parsePaymentReturnQuery(route.query)
    if (returnContext && isHostAllowed.value) {
      open('recharge', returnContext)
      const cleanupLocation = {
        path: route.path,
        query: stripPaymentReturnQuery(route.query) as LocationQueryRaw,
        hash: route.hash,
      }
      const expectedCleanup = {
        expectedFullPath: router.resolve(cleanupLocation).fullPath,
        publicToken: returnContext.publicToken,
      }
      pendingPaymentReturnCleanup = expectedCleanup
      void router.replace(cleanupLocation).catch(() => {
        if (
          pendingPaymentReturnCleanup?.expectedFullPath === expectedCleanup.expectedFullPath &&
          pendingPaymentReturnCleanup.publicToken === expectedCleanup.publicToken
        ) {
          pendingPaymentReturnCleanup = null
        }
      })
      return
    }

    if (routeChanged) close()
  },
  { immediate: true },
)

watch(
  () => userStore.user,
  (user) => {
    if (!user) {
      close()
    }
  },
)

watch(
  () => [isHostAllowed.value, visible.value] as const,
  ([allowed, isVisible]) => {
    if (!allowed && isVisible) {
      close()
    }
  },
)
</script>
