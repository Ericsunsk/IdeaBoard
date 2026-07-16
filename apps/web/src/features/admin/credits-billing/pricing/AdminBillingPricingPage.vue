<template>
  <AdminResourceGridFrame
    empty-description="注册后的计费功能会显示在这里。"
    empty-title="暂无可定价功能"
    :is-empty="!errorMessage && items.length === 0"
    :is-loading="isLoading"
  >
    <div v-if="errorMessage" :class="adminEmptyStateClass" role="alert">
      <div class="text-admin-title font-semibold text-admin-title-color">功能定价加载失败</div>
      <div class="mt-2 text-admin-meta text-admin-subtitle-color">{{ errorMessage }}</div>
      <Button
        class="mt-4"
        label="重新加载"
        severity="secondary"
        outlined
        :disabled="isLoading"
        @click="loadCatalog"
      >
        <template #icon><Refresh class="size-4" /></template>
      </Button>
    </div>

    <BillingFeaturePriceCard v-for="item in items" v-else :key="item.sku" :item="item" />
  </AdminResourceGridFrame>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { Refresh } from '@iconoir/vue'
import Button from 'primevue/button'
import AdminResourceGridFrame from '../../shared/components/AdminResourceGridFrame.vue'
import { adminEmptyStateClass } from '../../shared/lib/adminUiClasses'
import BillingFeaturePriceCard from './BillingFeaturePriceCard.vue'
import { createAdminBillableCatalogResource } from './billableCatalogResource'
import type { AdminBillableItemCard } from './billableCatalogTypes'

const resource = createAdminBillableCatalogResource()
const items = ref<AdminBillableItemCard[]>([])
const isLoading = ref(true)
const errorMessage = ref('')

async function loadCatalog() {
  isLoading.value = true
  errorMessage.value = ''
  try {
    items.value = await resource.list()
  } catch (error) {
    errorMessage.value = error instanceof Error && error.message ? error.message : '请稍后重试。'
  } finally {
    isLoading.value = false
  }
}

onMounted(loadCatalog)
</script>
