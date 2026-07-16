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

    <BillingFeaturePriceCard
      v-for="item in items"
      v-else
      :key="item.sku"
      :item="item"
      :availability-pending="availabilityPending[item.sku]"
      @availability="setAvailability"
      @publish="openPriceDialog"
    />
  </AdminResourceGridFrame>

  <BillingPriceDialog
    v-model:visible="priceDialogVisible"
    :item="selectedItem"
    :pending="publishPending"
    :error-message="publishError"
    @submit="publishPrice"
  />
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { Refresh } from '@iconoir/vue'
import Button from 'primevue/button'
import AdminResourceGridFrame from '../../shared/components/AdminResourceGridFrame.vue'
import { adminEmptyStateClass } from '../../shared/lib/adminUiClasses'
import { formatAdminDisplayError, useAdminToast } from '../../shared/lib/adminToast'
import BillingFeaturePriceCard from './BillingFeaturePriceCard.vue'
import BillingPriceDialog from './BillingPriceDialog.vue'
import { createAdminBillableCatalogResource } from './billableCatalogResource'
import type { AdminBillableItemCard } from './billableCatalogTypes'

const resource = createAdminBillableCatalogResource()
const items = ref<AdminBillableItemCard[]>([])
const isLoading = ref(true)
const errorMessage = ref('')
const selectedItem = ref<AdminBillableItemCard | null>(null)
const priceDialogVisible = ref(false)
const publishPending = ref(false)
const publishError = ref('')
const availabilityPending = reactive<Record<string, boolean>>({})
const toast = useAdminToast()

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

function openPriceDialog(item: AdminBillableItemCard) {
  selectedItem.value = item
  publishError.value = ''
  priceDialogVisible.value = true
}

async function publishPrice(unitCredits: number) {
  const item = selectedItem.value
  if (!item || publishPending.value) return
  publishPending.value = true
  publishError.value = ''
  try {
    const price = await resource.publishPrice(item.sku, unitCredits)
    item.currentPrice = price
    item.updatedAt = price.createdAt
    if (item.status === 'unpriced') item.status = 'active'
    priceDialogVisible.value = false
    toast.success('价格已发布')
  } catch (error) {
    publishError.value = formatAdminDisplayError(error, '发布价格失败，请重试')
  } finally {
    publishPending.value = false
  }
}

async function setAvailability(item: AdminBillableItemCard, enabled: boolean) {
  if (availabilityPending[item.sku] || item.status === 'unpriced' || item.status === 'archived') {
    return
  }
  availabilityPending[item.sku] = true
  try {
    const availability = await resource.setAvailability(item.sku, enabled)
    item.status = availability.status
    item.updatedAt = availability.updatedAt
    toast.success(enabled ? '功能已启用' : '功能已停用')
  } catch (error) {
    toast.error(formatAdminDisplayError(error, enabled ? '启用功能失败，请重试' : '停用功能失败，请重试'))
  } finally {
    availabilityPending[item.sku] = false
  }
}
</script>
