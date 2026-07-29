<template>
  <AdminDialogShell
    :visible="visible"
    :header="`价格历史 · ${item?.displayName ?? ''}`"
    size="form"
    content-layout="scrollable"
    @update:visible="emit('update:visible', $event)"
  >
    <div v-if="loading" class="flex min-h-40 items-center justify-center" role="status">
      <ProgressSpinner class="size-8" stroke-width="4" />
      <span class="sr-only">正在加载价格历史</span>
    </div>

    <div
      v-else-if="errorMessage"
      class="flex min-h-40 flex-col items-center justify-center text-center"
      role="alert"
    >
      <div class="text-admin-title-sm font-semibold text-admin-title-color">价格历史加载失败</div>
      <div class="mt-2 text-admin-meta text-ib-text-muted">{{ errorMessage }}</div>
      <Button class="mt-4" label="重新加载" severity="secondary" outlined @click="emit('retry')" />
    </div>

    <div
      v-else-if="prices.length === 0"
      class="flex min-h-40 flex-col items-center justify-center text-center"
    >
      <div class="text-admin-title-sm font-semibold text-admin-title-color">暂无价格历史</div>
      <div class="mt-2 text-admin-meta text-ib-text-muted">
        发布首个价格后，价格历史会显示在这里。
      </div>
    </div>

    <div v-else class="flex flex-col gap-2">
      <div
        v-for="price in prices"
        :key="price.priceVersionId"
        class="grid grid-cols-[minmax(0,1fr)_auto] gap-4 border-b border-ib-border-soft py-3 last:border-b-0"
      >
        <div class="min-w-0">
          <div class="flex items-center gap-2">
            <span class="text-admin-body font-semibold text-admin-title-color"
              >版本 {{ price.version }}</span
            >
            <Tag
              v-if="price.priceVersionId === item?.currentPrice?.priceVersionId"
              value="当前版本"
              severity="success"
              rounded
              :class="adminCompactTagClass"
            />
          </div>
          <div class="mt-1 text-admin-caption text-ib-text-muted">
            操作者 {{ price.createdBy }} · {{ formatAdminDateTime(price.createdAt) }}
          </div>
        </div>
        <div class="self-center text-right text-admin-body font-bold text-admin-title-color">
          {{ formatIdeaAmount(price.unitCredits) }}
        </div>
      </div>
      <Button
        v-if="hasMore"
        class="mt-2 self-center"
        label="加载更多"
        severity="secondary"
        text
        :loading="loadingMore"
        :disabled="loadingMore"
        @click="emit('load-more')"
      />
    </div>
  </AdminDialogShell>
</template>

<script setup lang="ts">
import Button from 'primevue/button'
import ProgressSpinner from 'primevue/progressspinner'
import Tag from 'primevue/tag'
import { formatAdminDateTime } from '../../../../shared/utils/dateTime'
import { formatIdeaAmount } from '../../../../shared/utils/ideaCurrency'
import AdminDialogShell from '../../shared/components/AdminDialogShell.vue'
import { adminCompactTagClass } from '../../shared/lib/adminUiClasses'
import type { AdminBillableItemCard, AdminBillableItemPrice } from './billableCatalogTypes'

withDefaults(
  defineProps<{
    visible: boolean
    item?: AdminBillableItemCard | null
    prices?: AdminBillableItemPrice[]
    loading?: boolean
    loadingMore?: boolean
    hasMore?: boolean
    errorMessage?: string
  }>(),
  {
    item: null,
    prices: () => [],
    loading: false,
    loadingMore: false,
    hasMore: false,
    errorMessage: '',
  },
)

const emit = defineEmits<{
  'update:visible': [visible: boolean]
  retry: []
  'load-more': []
}>()
</script>
