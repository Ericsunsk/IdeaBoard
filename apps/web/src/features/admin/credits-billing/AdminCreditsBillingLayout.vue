<template>
  <div class="flex min-h-0 min-w-0 flex-col gap-4">
    <div class="flex min-w-0 items-center justify-end gap-1.5">
      <Button
        v-tooltip.top="'Stripe 设置'"
        text
        rounded
        severity="secondary"
        size="small"
        class="!size-8 !p-0"
        aria-label="Stripe 设置"
        @click="stripeSettingsVisible = true"
      >
        <template #icon>
          <Settings class="size-4" />
        </template>
      </Button>
      <nav aria-label="积分运营">
        <SelectButton
          :model-value="activeNavigationPath"
          :options="navigationItems"
          option-label="label"
          option-value="path"
          :allow-empty="false"
          size="small"
          aria-label="积分运营页面"
          @update:model-value="navigate"
        >
          <template #option="{ option }">
            <span class="flex items-center gap-2">
              <span>{{ option.label }}</span>
              <span
                v-if="option.key === 'health' && healthBadge"
                class="rounded-[var(--radius-ib-pill)] border px-1.5 py-0.5 text-admin-caption leading-none font-bold"
                :class="healthBadge.className"
              >
                {{ healthBadge.label }}
              </span>
            </span>
          </template>
        </SelectButton>
      </nav>
    </div>
    <RouterView />
    <StripeSettingsDialog v-model:visible="stripeSettingsVisible" />
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { RouterView, useRoute, useRouter } from 'vue-router'
import { Settings } from '@iconoir/vue'
import Button from 'primevue/button'
import SelectButton from 'primevue/selectbutton'
import Tooltip from 'primevue/tooltip'
import {
  ADMIN_CREDIT_HEALTH_PATH,
  ADMIN_CREDIT_PRICING_PATH,
  ADMIN_PAYMENT_ORDERS_PATH,
  ADMIN_REWARD_CODES_PATH,
  isPathUnderRoute,
} from '../../../shared/constants/routePaths'
import { useAdminCreditHealthResource } from './health/adminCreditHealthResource'
import { getCreditHealthPresentation } from './health/creditHealthPresentation'
import StripeSettingsDialog from '../payments/stripe-settings/StripeSettingsDialog.vue'

const vTooltip = Tooltip

type NavigationPath =
  | typeof ADMIN_CREDIT_PRICING_PATH
  | typeof ADMIN_CREDIT_HEALTH_PATH
  | typeof ADMIN_REWARD_CODES_PATH
  | typeof ADMIN_PAYMENT_ORDERS_PATH

const navigationItems: Array<{ key: string; path: NavigationPath; label: string }> = [
  { key: 'pricing', path: ADMIN_CREDIT_PRICING_PATH, label: '功能定价' },
  { key: 'health', path: ADMIN_CREDIT_HEALTH_PATH, label: '账本健康' },
  { key: 'rewards', path: ADMIN_REWARD_CODES_PATH, label: '兑换码' },
  { key: 'payments', path: ADMIN_PAYMENT_ORDERS_PATH, label: '支付订单' },
]

const route = useRoute()
const router = useRouter()
const stripeSettingsVisible = ref(false)
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
          ? 'border-admin-danger text-admin-danger'
          : presentation.tone === 'warning'
            ? 'border-admin-warning text-admin-warning'
            : 'border-admin-border text-admin-text-3',
    }
  }
  return healthResource.query.isError.value
    ? { label: '未知', className: 'border-admin-border text-admin-text-3' }
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
