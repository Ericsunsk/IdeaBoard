<template>
  <div class="flex flex-col gap-3">
    <!-- 头部：板块名 + 副标题，配置按钮右上 -->
    <div class="flex flex-wrap items-center justify-between gap-3 pb-1">
      <div class="flex min-w-0 flex-col gap-0.5">
        <div class="flex items-center gap-2">
          <component
            :is="SETTINGS_SECTIONS.payment.icon"
            class="size-4 shrink-0 text-muted-foreground"
            aria-hidden="true"
          />
          <h3 class="text-admin-title-sm font-bold text-foreground">
            {{ SETTINGS_SECTIONS.payment.title }}
          </h3>
        </div>
        <p class="text-admin-caption text-muted-foreground">
          {{ M.paymentDescription }}
        </p>
      </div>
      <ActionButton :disabled="query.isPending.value" @click="visible = true">{{
        M.paymentConfigure
      }}</ActionButton>
    </div>

    <div class="flex flex-col gap-3 rounded-panel border border-border bg-card p-5">
      <div v-if="query.isPending.value" class="flex items-center gap-2" role="status">
        <AdminSpinner class="size-4" :aria-label="M.settingsLoading" />
        <span class="text-admin-body text-muted-foreground">{{ M.settingsLoading }}</span>
      </div>
      <template v-else>
        <div v-for="row in rows" :key="row.key" class="flex items-center justify-between gap-3">
          <span class="text-admin-body font-medium text-foreground">{{ row.label }}</span>
          <StatusBadge :label="row.status" :tone="row.tone" />
        </div>
      </template>
    </div>

    <StripeSettingsDialog v-model:visible="visible" />
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useQuery } from '@tanstack/vue-query'
import ActionButton from '@/core/components/ActionButton.vue'
import { useFailureToast } from '@/core/composables/useFailureToast'
import { AdminSpinner } from '../../../../shared/index'
import { StatusBadge } from '../../../../shared/index'
import StripeSettingsDialog from './StripeSettingsDialog.vue'
import { createAdminStripeSettingsResource } from '../../../session/payment/resource'
import { formatAdminDisplayError } from '../../../../shared/index'
import { PAYMENTS_MESSAGES } from '../../../session/payment/messages'
import { SETTINGS_MESSAGES as M } from '../../pages/messages'
import { SETTINGS_SECTIONS } from '../../pages/sections'
import { useRefetchOnClose } from '../../../../shared/index'

const visible = ref(false)

const query = useQuery({
  queryKey: ['admin', 'settings', 'stripe'],
  queryFn: () => createAdminStripeSettingsResource().get(),
})

// 加载失败一律走 toast（与 useSettingsLifecycle.loadFailed 同公约），不设内嵌 Alert。
useFailureToast(
  query.error,
  (error) => formatAdminDisplayError(error, PAYMENTS_MESSAGES.stripeLoadFailed),
  { shouldFireOnMount: false },
)

const summary = computed(() => query.data.value)
// 方案 A：收款配置是配置态（占位/空态），走中性避免蹭健康色；
// Product/Webhook 是健康态：Ready=正常(success)、Not ready=未验证(neutral)。
const rows = computed(() => [
  {
    key: 'configured',
    label: '收款配置',
    status: M.paymentConfiguredState(summary.value?.isConfigured ?? false),
    tone: 'neutral' as const,
  },
  {
    key: 'product',
    label: '商品税率',
    status: PAYMENTS_MESSAGES.productReadyState(summary.value?.isProductReady ?? false),
    tone: summary.value?.isProductReady ? ('success' as const) : ('neutral' as const),
  },
  {
    key: 'webhook',
    label: 'Webhook',
    status: PAYMENTS_MESSAGES.webhookReadyState(summary.value?.isWebhookReady ?? false),
    tone: summary.value?.isWebhookReady ? ('success' as const) : ('neutral' as const),
  },
])

// 弹窗内保存后关闭时刷新摘要，保持板块状态与弹窗一致。
useRefetchOnClose(visible, () => query.refetch())
</script>
