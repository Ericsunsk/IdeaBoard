<template>
  <div :class="adminDashboardRootClass">
    <!-- 1. 顶栏操作与搜索过滤工具条 (1:1 复刻模型管理页与系统提示词页规范) -->
    <div v-if="!errorMessage" class="flex w-full flex-col gap-3">
      <div class="flex w-full flex-wrap items-center justify-between gap-2.5">
        <!-- 左侧：搜索输入框与状态筛选器 -->
        <div class="flex min-w-0 flex-1 flex-wrap items-center gap-2">
          <AdminToolbarSearch
            v-model="searchKeyword"
            fluid
            :input-label="BILLING_PRICING_MESSAGES.searchLabel"
            :placeholder="BILLING_PRICING_MESSAGES.searchPlaceholder"
          />

          <AdminOptionFilter
            v-model="statusFilter"
            :options="statusFilterOptions"
            :label="BILLING_PRICING_MESSAGES.filterLabel"
          />
        </div>

        <!-- 右侧：刷新同步按钮 -->
        <div class="flex shrink-0 items-center gap-2">
          <AdminRefreshButton
            :loading="isLoading"
            :aria-label="BILLING_PRICING_MESSAGES.refresh"
            @click="loadCatalog"
          />
        </div>
      </div>

      <!-- 次级统计提示条：共 X 个计费功能 -->
      <div v-if="!isLoading" class="flex items-center justify-between pt-1">
        <div class="flex items-center gap-2 text-sm text-muted-foreground">
          <span class="font-medium text-foreground">{{
            BILLING_PRICING_MESSAGES.billingCount(filteredItems.length)
          }}</span>
          <span v-if="hasActiveFilter" class="text-xs text-muted-foreground">
            {{ BILLING_PRICING_MESSAGES.totalCount(sortedItems.length) }}
          </span>
        </div>
      </div>
    </div>

    <!-- 2. 加载中状态 (AdminSpinner 自带 role="progressbar") -->
    <div
      v-if="isLoading && items.length === 0"
      class="flex min-h-[260px] items-center justify-center rounded-xl border border-border bg-card text-muted-foreground"
      :aria-label="BILLING_PRICING_MESSAGES.loadingAria"
    >
      <AdminSpinner class="size-9 text-primary" />
    </div>

    <!-- 3. 错误告警状态 -->
    <div v-else-if="errorMessage" :class="adminEmptyStateClass" role="alert">
      <div class="text-admin-title font-semibold text-foreground">
        {{ BILLING_PRICING_MESSAGES.loadFailedTitle }}
      </div>
      <div class="mt-2 text-admin-meta text-muted-foreground">{{ errorMessage }}</div>
      <AdminButton class="mt-4" variant="outline" :disabled="isLoading" @click="loadCatalog">
        <Refresh class="size-4" />
        {{ BILLING_PRICING_MESSAGES.reload }}
      </AdminButton>
    </div>

    <!-- 4. 空状态提示 (无数据或过滤未命中) -->
    <div
      v-else-if="filteredItems.length === 0"
      class="flex min-h-[320px] flex-col items-center justify-center rounded-xl border border-dashed border-border bg-card p-10 text-center"
    >
      <div
        class="mb-3.5 flex size-12 items-center justify-center rounded-2xl bg-muted text-muted-foreground"
      >
        <Coins v-if="!hasActiveFilter" class="size-6" />
        <Search v-else class="size-6" />
      </div>
      <h3 class="text-base font-semibold text-foreground">
        {{
          !hasActiveFilter
            ? BILLING_PRICING_MESSAGES.emptyNone
            : BILLING_PRICING_MESSAGES.emptyNoMatch
        }}
      </h3>
      <p class="mt-1 max-w-sm text-xs text-muted-foreground">
        {{
          !hasActiveFilter
            ? BILLING_PRICING_MESSAGES.emptyNoneHint
            : BILLING_PRICING_MESSAGES.emptyNoMatchHint
        }}
      </p>
      <button
        v-if="hasActiveFilter"
        type="button"
        class="mt-4 flex h-8 items-center gap-1.5 rounded-lg border border-border bg-card px-3 text-xs font-medium text-foreground transition-colors hover:bg-muted cursor-pointer"
        @click="clearFilters"
      >
        <Xmark class="size-3.5" />
        <span>{{ BILLING_PRICING_MESSAGES.clearFilters }}</span>
      </button>
    </div>

    <!-- 5. 分 namespace 定价表（Image / Other，ai-search 暂归 Other，对齐任务监控页事件表） -->
    <div v-else class="flex flex-col gap-6">
      <BillingPricingTable
        v-for="section in pricedSections"
        :key="section.key"
        :title="section.title"
        :items="section.items"
        :loading="isLoading"
        :availability-pending="availabilityPending"
        :delete-pending="deletePending"
        :pool-by-sku="poolBySku"
        :pool-failed="poolFailed"
        @availability="setAvailability"
        @publish="openPriceDialog"
        @delete="confirmArchivedDelete"
      >
      </BillingPricingTable>
    </div>

    <!-- 6. 统一服务设置弹窗（定价与模型池） -->
    <BillingPriceDialog
      v-model:visible="priceDialogVisible"
      :item="selectedItem"
      :pending="publishPending"
      :error-message="publishError"
      :pool-version="poolEditVersion"
      :pool-entries="poolEditEntries"
      :pool-catalog-models="poolCatalogModels"
      :pool-catalog-loading="poolCatalogLoading"
      :pool-save-pending="poolSavePending"
      :pool-save-error="poolSaveError"
      :pool-save-stale="poolSaveStale"
      :pool-save-disabled="!poolEditReady"
      :pool-catalog-error="poolCatalogError"
      @submit="publishPrice"
      @submit-pool="savePool"
      @reload-pool="reloadPool"
      @reload-catalog="loadPoolCatalog"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { Coins, Refresh, Search, Xmark } from '@iconoir/vue'
import AdminButton from '../../shared/components/AdminButton.vue'
import AdminOptionFilter from '../../shared/components/AdminOptionFilter.vue'
import AdminRefreshButton from '../../shared/components/AdminRefreshButton.vue'
import AdminSpinner from '../../shared/components/AdminSpinner.vue'
import AdminToolbarSearch from '../../shared/components/AdminToolbarSearch.vue'
import { adminDashboardRootClass, adminEmptyStateClass } from '../../shared/lib/uiClasses'
import { formatAdminDisplayError, useAdminToast } from '../../shared/lib/toast'
import BillingPricingTable from './BillingPricingTable.vue'
import BillingPriceDialog from './BillingPriceDialog.vue'
import { useAdminConfirm } from '../../shared/composables/useAdminConfirm'
import { confirmSettingsSave, requireDestructiveReason } from '../../shared/lib/confirmSettingsSave'
import { createAdminBillableCatalogResource } from './billableCatalogResource'
import { createAdminModelPoolResource } from './modelPoolResource'
import { serviceOfSku } from './skuFeature'
import {
  PRICING_NAMESPACE_TABLES,
  PRICING_OTHER_TABLE,
  pricingNamespaceRank,
  pricingTableKeyOfSku,
} from './groups'
import type {
  AdminBillableItemCard,
  AdminModelPool,
  AdminModelPoolEntry,
  AdminPoolCatalogModel,
  PricingVariant,
} from './billableCatalogTypes'
import { BILLING_PRICING_MESSAGES } from './messages'

const resource = createAdminBillableCatalogResource()
const poolResource = createAdminModelPoolResource()
const items = ref<AdminBillableItemCard[]>([])
const sortedItems = computed(() => [...items.value].sort(compareCatalogCards))

// 每 Service 模型池快照（key 为 billing SKU）：定价表摘要列与编辑器共享。
// 加载失败按 SKU 标记，摘要列就地展示失败态，刷新按钮可整体重试。
const poolBySku = ref<Record<string, AdminModelPool>>({})
const poolFailed = reactive<Record<string, boolean>>({})
const poolEditItem = ref<AdminBillableItemCard | null>(null)
const poolEditVersion = ref(0)
const poolEditEntries = ref<AdminModelPoolEntry[]>([])
const poolEditReady = ref(false)
const poolCatalogModels = ref<AdminPoolCatalogModel[]>([])
const poolCatalogLoading = ref(false)
const poolCatalogError = ref('')
const poolSavePending = ref(false)
const poolSaveError = ref('')
const poolSaveStale = ref(false)

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

// 搜索与状态筛选
const searchKeyword = ref('')
const statusFilter = ref('')

const statusFilterOptions = [
  { label: BILLING_PRICING_MESSAGES.statusAll, value: '' },
  { label: BILLING_PRICING_MESSAGES.statusActive, value: 'active' },
  { label: BILLING_PRICING_MESSAGES.statusUnpriced, value: 'unpriced' },
  { label: BILLING_PRICING_MESSAGES.statusDisabled, value: 'disabled' },
  { label: BILLING_PRICING_MESSAGES.statusArchived, value: 'archived' },
]

const hasActiveFilter = computed(() => Boolean(searchKeyword.value.trim() || statusFilter.value))

function clearFilters() {
  searchKeyword.value = ''
  statusFilter.value = ''
}

// 过滤后的列表
const filteredItems = computed(() => {
  const query = searchKeyword.value.trim().toLowerCase()
  const filter = statusFilter.value

  return sortedItems.value.filter((item) => {
    if (filter && item.status !== filter) return false
    if (!query) return true

    if (item.displayName.toLowerCase().includes(query)) return true
    if (item.sku.toLowerCase().includes(query)) return true

    return false
  })
})

function compareCatalogCards(left: AdminBillableItemCard, right: AdminBillableItemCard) {
  const namespaceDifference = pricingNamespaceRank(left.sku) - pricingNamespaceRank(right.sku)
  if (namespaceDifference !== 0) return namespaceDifference
  const priorityDifference = catalogCardPriority(left) - catalogCardPriority(right)
  if (priorityDifference !== 0) return priorityDifference
  if (left.sortOrder !== right.sortOrder) return left.sortOrder - right.sortOrder
  return left.sku.localeCompare(right.sku)
}

// 分 namespace 定价区：Image / Other，空组不渲染。
const pricedSections = computed(() => {
  const groups = [...PRICING_NAMESPACE_TABLES, PRICING_OTHER_TABLE]
  return groups
    .map((group) => ({
      key: group.namespace,
      title: group.title,
      items: filteredItems.value.filter(
        (item) => pricingTableKeyOfSku(item.sku) === group.namespace,
      ),
    }))
    .filter((section) => section.items.length > 0)
})

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
    await loadModelPools()
  } catch (error) {
    errorMessage.value =
      error instanceof Error && error.message ? error.message : BILLING_PRICING_MESSAGES.retryLater
  } finally {
    isLoading.value = false
  }
}

// 模型池摘要与目录加载解耦：定价失败仍整体报错；池按 ai-image SKU 并行
// 拉取，单池失败只污染该行摘要（poolFailed），不阻塞整页。
async function loadModelPools() {
  const skus = sortedItems.value.map((item) => item.sku).filter((sku) => serviceOfSku(sku))
  const settled = await Promise.allSettled(skus.map((sku) => poolResource.get(sku)))
  settled.forEach((result, index) => {
    const sku = skus[index]!
    if (result.status === 'fulfilled') {
      poolBySku.value[sku] = result.value
      poolFailed[sku] = false
    } else {
      poolFailed[sku] = true
    }
  })
}

async function loadPoolCatalog() {
  if (poolCatalogLoading.value) return
  poolCatalogLoading.value = true
  poolCatalogError.value = ''
  try {
    poolCatalogModels.value = await poolResource.listCatalogModels()
  } catch (error) {
    poolCatalogModels.value = []
    poolCatalogError.value = formatAdminDisplayError(error, '加载模型目录失败，请重试')
  } finally {
    poolCatalogLoading.value = false
  }
}

async function reloadPool() {
  const item = poolEditItem.value
  if (!item) return
  poolEditReady.value = false
  poolSavePending.value = true
  poolSaveError.value = ''
  poolSaveStale.value = false
  try {
    const pool = await poolResource.get(item.sku)
    poolBySku.value[item.sku] = pool
    poolFailed[item.sku] = false
    poolEditVersion.value = pool.version
    poolEditEntries.value = pool.entries.map((entry) => ({
      modelId: entry.modelId,
      enabled: entry.enabled,
      resolutions: [...entry.resolutions],
    }))
    poolEditReady.value = true
  } catch (error) {
    poolEditVersion.value = 0
    poolEditEntries.value = []
    poolEditReady.value = false
    poolSaveError.value = formatAdminDisplayError(error, BILLING_PRICING_MESSAGES.poolSaveFailed)
    poolSaveStale.value = true
  } finally {
    poolSavePending.value = false
  }
}

async function savePool(entries: AdminModelPoolEntry[]) {
  const item = poolEditItem.value
  if (!item || poolSavePending.value || !poolEditReady.value) return
  // Control writes require an operator reason (audit); a cancelled dialog
  // aborts silently. The save replaces the complete Service pool
  // atomically against the loaded version (409 on concurrent edits).
  const operatorReason = await confirmSettingsSave(confirm, {
    title: BILLING_PRICING_MESSAGES.poolSaveTitle,
    message: BILLING_PRICING_MESSAGES.poolSaveMessage(item.displayName),
    confirmLabel: BILLING_PRICING_MESSAGES.poolSaveConfirm,
  })
  if (!operatorReason) return
  poolSavePending.value = true
  poolSaveError.value = ''
  poolSaveStale.value = false
  try {
    const saved = await poolResource.replace(
      item.sku,
      { version: poolEditVersion.value, entries },
      { operatorReason },
    )
    poolBySku.value[item.sku] = saved
    poolFailed[item.sku] = false
    poolEditVersion.value = saved.version
    poolEditEntries.value = saved.entries.map((entry) => ({
      modelId: entry.modelId,
      enabled: entry.enabled,
      resolutions: [...entry.resolutions],
    }))
    priceDialogVisible.value = false
    toast.success(BILLING_PRICING_MESSAGES.poolSaved)
  } catch (error) {
    poolSaveError.value = formatAdminDisplayError(error, BILLING_PRICING_MESSAGES.poolSaveFailed)
    poolSaveStale.value =
      error instanceof Error && (error as { code?: unknown }).code === 'conflict'
  } finally {
    poolSavePending.value = false
  }
}

onMounted(loadCatalog)

async function openPriceDialog(item: AdminBillableItemCard) {
  selectedItem.value = item
  publishError.value = ''
  priceDialogVisible.value = true
  if (serviceOfSku(item.sku)) {
    poolEditItem.value = item
    poolEditReady.value = false
    poolSaveError.value = ''
    poolSaveStale.value = false
    await reloadPool()
    if (poolCatalogModels.value.length === 0 && !poolCatalogLoading.value) {
      await loadPoolCatalog()
    }
  }
}

async function confirmArchivedDelete(item: AdminBillableItemCard) {
  if (item.status !== 'archived' || deletePending[item.sku]) return
  const confirmed = await confirm.confirm({
    title: BILLING_PRICING_MESSAGES.deleteTitle,
    message: BILLING_PRICING_MESSAGES.deleteMessage(item.displayName),
    confirmLabel: BILLING_PRICING_MESSAGES.deleteConfirm,
    intent: 'danger',
    reason: { required: true },
  })
  if (!confirmed) return
  const operatorReason = requireDestructiveReason(confirm)
  if (!operatorReason) return
  await deleteArchived(item, operatorReason)
}

async function deleteArchived(item: AdminBillableItemCard, operatorReason: string) {
  if (deletePending[item.sku]) return
  deletePending[item.sku] = true
  try {
    await resource.deleteArchived(item.sku, { operatorReason })
    items.value = items.value.filter((candidate) => candidate.sku !== item.sku)
    toast.success(BILLING_PRICING_MESSAGES.deleted)
  } catch (error) {
    toast.error(formatAdminDisplayError(error, BILLING_PRICING_MESSAGES.deleteFailed))
  } finally {
    deletePending[item.sku] = false
  }
}

async function publishPrice(prices: Record<PricingVariant, number | undefined>) {
  const item = selectedItem.value
  if (!item || publishPending.value) return
  // Control writes require an operator reason (audit); a cancelled dialog
  // aborts silently.
  const operatorReason = await confirmSettingsSave(confirm, {
    title: BILLING_PRICING_MESSAGES.publishTitle,
    message: BILLING_PRICING_MESSAGES.publishMessage(item.displayName),
    confirmLabel: BILLING_PRICING_MESSAGES.publishConfirm,
  })
  if (!operatorReason) return
  publishPending.value = true
  publishError.value = ''
  try {
    const published = await resource.publishPrices(item.sku, prices, { operatorReason })
    item.currentPrices = published
    const latest = Object.values(published).sort((a, b) =>
      b.createdAt.localeCompare(a.createdAt),
    )[0]
    if (latest) item.updatedAt = latest.createdAt
    if (item.status === 'unpriced') item.status = 'active'
    priceDialogVisible.value = false
    toast.success(BILLING_PRICING_MESSAGES.published)
  } catch (error) {
    publishError.value = formatAdminDisplayError(error, BILLING_PRICING_MESSAGES.publishFailed)
  } finally {
    publishPending.value = false
  }
}

async function setAvailability(item: AdminBillableItemCard, enabled: boolean) {
  if (availabilityPending[item.sku] || item.status === 'unpriced' || item.status === 'archived') {
    return
  }
  const operatorReason = await confirmSettingsSave(confirm, {
    title: enabled ? BILLING_PRICING_MESSAGES.enableTitle : BILLING_PRICING_MESSAGES.disableTitle,
    message: BILLING_PRICING_MESSAGES.availabilityMessage(enabled, item.displayName),
    confirmLabel: enabled ? BILLING_PRICING_MESSAGES.enable : BILLING_PRICING_MESSAGES.disable,
  })
  if (!operatorReason) return
  availabilityPending[item.sku] = true
  try {
    const availability = await resource.setAvailability(item.sku, enabled, { operatorReason })
    item.status = availability.status
    item.updatedAt = availability.updatedAt
    toast.success(
      enabled ? BILLING_PRICING_MESSAGES.enabledToast : BILLING_PRICING_MESSAGES.disabledToast,
    )
  } catch (error) {
    toast.error(
      formatAdminDisplayError(
        error,
        enabled ? BILLING_PRICING_MESSAGES.enableFailed : BILLING_PRICING_MESSAGES.disableFailed,
      ),
    )
  } finally {
    availabilityPending[item.sku] = false
  }
}
</script>
