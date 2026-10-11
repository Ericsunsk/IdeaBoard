<template>
  <AdminSubNavigationLayout
    :items="navigationItems"
    :active-path="activeNavigationPath"
    @navigate="navigate($event as NavigationPath)"
  >
    <template #item="{ item }">
      <span>{{ item.label }}</span>
      <span
        v-if="item.key === 'health' && healthBadge"
        class="rounded-full border px-1.5 py-0.5 text-admin-caption leading-none font-bold"
        :class="healthBadge.className"
      >
        {{ healthBadge.label }}
      </span>
    </template>
  </AdminSubNavigationLayout>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { AdminSubNavigationLayout } from '../../shared'
import {
  ADMIN_CREDIT_HEALTH_PATH,
  ADMIN_CREDIT_PRICING_PATH,
  ADMIN_PAYMENT_ORDERS_PATH,
  ADMIN_REWARD_CODES_PATH,
  isPathUnderRoute,
} from '../../routePaths'
import { useAdminCreditHealthResource } from '../session/health/resource'
import { getCreditHealthPresentation } from '../core/health/creditHealthPresentation'
import { CREDITS_BILLING_LAYOUT_MESSAGES } from './messages'

type NavigationPath =
  | typeof ADMIN_CREDIT_PRICING_PATH
  | typeof ADMIN_CREDIT_HEALTH_PATH
  | typeof ADMIN_REWARD_CODES_PATH
  | typeof ADMIN_PAYMENT_ORDERS_PATH

const navigationItems: Array<{ key: string; path: NavigationPath; label: string }> = [
  {
    key: 'pricing',
    path: ADMIN_CREDIT_PRICING_PATH,
    label: CREDITS_BILLING_LAYOUT_MESSAGES.navPricing,
  },
  {
    key: 'health',
    path: ADMIN_CREDIT_HEALTH_PATH,
    label: CREDITS_BILLING_LAYOUT_MESSAGES.navHealth,
  },
  {
    key: 'rewards',
    path: ADMIN_REWARD_CODES_PATH,
    label: CREDITS_BILLING_LAYOUT_MESSAGES.navRewards,
  },
  {
    key: 'payments',
    path: ADMIN_PAYMENT_ORDERS_PATH,
    label: CREDITS_BILLING_LAYOUT_MESSAGES.navPayments,
  },
]

const route = useRoute()
const router = useRouter()

const healthResource = useAdminCreditHealthResource()
const healthBadge = computed(() => {
  const status = healthResource.health.value?.status
  if (status) {
    const presentation = getCreditHealthPresentation(status)
    if (!presentation.badgeLabel) return null
    return {
      label: presentation.badgeLabel,
      className:
        presentation.tone === 'danger'
          ? 'border-destructive text-destructive'
          : presentation.tone === 'warning'
            ? 'border-warning text-warning'
            : 'border-border text-muted-foreground',
    }
  }
  return healthResource.query.isError.value
    ? {
        label: CREDITS_BILLING_LAYOUT_MESSAGES.navUnknownBadge,
        className: 'border-border text-muted-foreground',
      }
    : null
})
const activeNavigationPath = computed<NavigationPath>(
  () =>
    navigationItems.find((item) => isPathUnderRoute(route.path, item.path))?.path ??
    ADMIN_CREDIT_PRICING_PATH,
)

function navigate(path: NavigationPath) {
  if (isPathUnderRoute(route.path, path)) return
  void router.push(path)
}
</script>
