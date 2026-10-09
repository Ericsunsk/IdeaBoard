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

    <AdminAlert v-if="query.error.value" severity="error">
      {{ PAYMENTS_MESSAGES.stripeLoadFailed }}
      <AdminButton variant="outline" @click="query.refetch()">{{ M.retry }}</AdminButton>
    </AdminAlert>

    <div class="flex flex-col gap-3 rounded-xl border border-border bg-card p-5">
      <div v-if="query.isPending.value" class="flex items-center gap-2" role="status">
        <AdminSpinner class="size-4" :aria-label="M.settingsLoading" />
        <span class="text-admin-body text-muted-foreground">{{ M.settingsLoading }}</span>
      </div>
      <template v-else>
        <div v-for="row in rows" :key="row.key" class="flex items-center justify-between gap-3">
          <span class="text-admin-body font-medium text-foreground">{{ row.label }}</span>
          <AdminStatusPill :label="row.status" :variant="row.ready ? 'success' : 'neutral'" />
        </div>
      </template>
    </div>

    <StripeSettingsDialog v-model:visible="visible" />
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useQuery } from '@tanstack/vue-query'
import ActionButton from '../../../../shared/components/ActionButton.vue'
import AdminAlert from '../../shared/components/AdminAlert.vue'
import AdminButton from '../../shared/components/AdminButton.vue'
import AdminSpinner from '../../shared/components/AdminSpinner.vue'
import AdminStatusPill from '../../shared/components/AdminStatusPill.vue'
import StripeSettingsDialog from '../../payments/stripe-settings/StripeSettingsDialog.vue'
import { createAdminStripeSettingsResource } from '../../payments/stripe-settings/resource'
import { PAYMENTS_MESSAGES } from '../../payments/messages'
import { SETTINGS_MESSAGES as M } from '../pages/messages'
import { SETTINGS_SECTIONS } from '../shared/sections'

const visible = ref(false)

const query = useQuery({
  queryKey: ['admin', 'settings', 'stripe'],
  queryFn: () => createAdminStripeSettingsResource().get(),
})

const summary = computed(() => query.data.value)
const rows = computed(() => [
  {
    key: 'configured',
    label: '收款配置',
    status: M.paymentConfiguredState(summary.value?.configured ?? false),
    ready: summary.value?.configured ?? false,
  },
  {
    key: 'product',
    label: '商品税率',
    status: PAYMENTS_MESSAGES.productReadyState(summary.value?.productReady ?? false),
    ready: summary.value?.productReady ?? false,
  },
  {
    key: 'webhook',
    label: 'Webhook',
    status: PAYMENTS_MESSAGES.webhookReadyState(summary.value?.webhookReady ?? false),
    ready: summary.value?.webhookReady ?? false,
  },
])

// 弹窗内保存后关闭时刷新摘要，保持板块状态与弹窗一致。
watch(visible, (open) => {
  if (!open) void query.refetch()
})
</script>
