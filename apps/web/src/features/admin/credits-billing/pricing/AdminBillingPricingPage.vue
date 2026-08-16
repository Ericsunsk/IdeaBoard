<template>
  <AdminResourceGridFrame
    empty-description="注册后的计费功能会显示在这里。"
    empty-title="暂无可定价功能"
    grid-class="grid grid-cols-1 items-start gap-3.5 md:grid-cols-2 xl:grid-cols-3"
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
      v-for="item in sortedItems"
      v-else
      :key="item.sku"
      :item="item"
      :availability-pending="availabilityPending[item.sku]"
      :delete-pending="deletePending[item.sku]"
      @availability="setAvailability"
      @publish="openPriceDialog"
      @delete="confirmArchivedDelete"
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
import { computed, onMounted, reactive, ref } from 'vue'
import { Refresh } from '@iconoir/vue'
import Button from 'primevue/button'
import AdminResourceGridFrame from '../../shared/components/AdminResourceGridFrame.vue'
import { adminEmptyStateClass } from '../../shared/lib/adminUiClasses'
import { formatAdminDisplayError, useAdminToast } from '../../shared/lib/adminToast'
import BillingFeaturePriceCard from './BillingFeaturePriceCard.vue'
import BillingPriceDialog from './BillingPriceDialog.vue'
import { useAdminConfirm } from '../../shared/composables/useAdminConfirm'
import { createAdminBillableCatalogResource } from './billableCatalogResource'
import type { AdminBillableItemCard, PricingVariant } from './billableCatalogTypes'

const resource = createAdminBillableCatalogResource()
const items = ref<AdminBillableItemCard[]>([])
const sortedItems = computed(() => [...items.value].sort(compareCatalogCards))
const isLoading = ref(true)
const errorMessage = ref('')
const selectedItem = ref<AdminBillableItemCard | null>(null)
const priceDialogVisible = ref(false)
const publishPending = ref(false)
const publishError = ref('')
const availabilityPending = reactive<Record<string, boolean>>({})
const deletePending = reactive<Record<string, boolean>>({})
const toast = useAdminToast()
const confirm = useAdminConfirm()

function compareCatalogCards(left: AdminBillableItemCard, right: AdminBillableItemCard) {
  const priorityDifference = catalogCardPriority(left) - catalogCardPriority(right)
  if (priorityDifference !== 0) return priorityDifference
  if (left.sortOrder !== right.sortOrder) return left.sortOrder - right.sortOrder
  return left.sku.localeCompare(right.sku)
}

function catalogCardPriority(item: AdminBillableItemCard) {
  if (item.status === 'archived') return 4
  const hasConfiguredPrice = item.pricingVariants.some(
    (variant) => item.currentPrices[variant] !== undefined,
  )
  if (item.status === 'active' && hasConfiguredPrice) return 0
  if (item.pricingVariants.length === 1) return 1
  if (hasConfiguredPrice) return 2
  return 3
}

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

function confirmArchivedDelete(item: AdminBillableItemCard) {
  if (item.status !== 'archived' || deletePending[item.sku]) return
  confirm.request({
    title: '删除已归档功能',
    message: `确定彻底删除“${item.displayName}”及其价格配置吗？此操作不可撤销。`,
    confirmLabel: '确认删除',
    intent: 'danger',
    onConfirm: () => deleteArchived(item),
  })
}

async function deleteArchived(item: AdminBillableItemCard) {
  if (deletePending[item.sku]) return
  deletePending[item.sku] = true
  try {
    await resource.deleteArchived(item.sku)
    items.value = items.value.filter((candidate) => candidate.sku !== item.sku)
    toast.success('已归档功能已删除')
  } catch (error) {
    toast.error(formatAdminDisplayError(error, '删除已归档功能失败，请重试'))
  } finally {
    deletePending[item.sku] = false
  }
}

async function publishPrice(prices: Record<PricingVariant, number | undefined>) {
  const item = selectedItem.value
  if (!item || publishPending.value) return
  publishPending.value = true
  publishError.value = ''
  try {
    const published = await resource.publishPrices(item.sku, prices)
    item.currentPrices = published
    const latest = Object.values(published).sort((a, b) =>
      b.createdAt.localeCompare(a.createdAt),
    )[0]
    if (latest) item.updatedAt = latest.createdAt
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
    toast.error(
      formatAdminDisplayError(error, enabled ? '启用功能失败，请重试' : '停用功能失败，请重试'),
    )
  } finally {
    availabilityPending[item.sku] = false
  }
}
</script>
