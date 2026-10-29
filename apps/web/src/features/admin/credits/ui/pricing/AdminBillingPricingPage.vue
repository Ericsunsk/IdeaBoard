<template>
  <div :class="adminDashboardRootClass">
    <!-- 1. 顶栏操作与搜索过滤工具条 (1:1 复刻模型管理页与系统提示词页规范) -->
    <div class="flex w-full flex-col gap-3">
      <div class="flex w-full flex-wrap items-center justify-between gap-2.5">
        <!-- 左侧：搜索输入框 -->
        <div class="flex min-w-0 flex-1 flex-wrap items-center gap-2">
          <AdminToolbarSearch
            v-model="searchKeyword"
            fluid
            :input-label="BILLING_PRICING_MESSAGES.searchLabel"
            :placeholder="BILLING_PRICING_MESSAGES.searchPlaceholder"
          />
        </div>

        <!-- 右侧：刷新同步按钮 -->
        <div class="shrink-0">
          <AdminRefreshButton
            :loading="isLoading"
            :aria-label="BILLING_PRICING_MESSAGES.refresh"
            @click="loadCatalog"
          />
        </div>
      </div>
    </div>

    <!-- 2. 加载中状态 (AdminSpinner 自带 role="progressbar") -->
    <div
      v-if="isLoading && items.length === 0"
      class="flex min-h-admin-empty-state items-center justify-center rounded-panel border border-border bg-card text-muted-foreground"
      :aria-label="BILLING_PRICING_MESSAGES.loadingAria"
    >
      <AdminSpinner class="size-9 text-primary" />
    </div>

    <!-- 3. 空状态提示 (无数据或过滤未命中) -->
    <AdminEmptyState
      v-else-if="filteredItems.length === 0"
      :title="
        !hasActiveFilter
          ? BILLING_PRICING_MESSAGES.emptyNone
          : BILLING_PRICING_MESSAGES.emptyNoMatch
      "
      :description="
        !hasActiveFilter
          ? BILLING_PRICING_MESSAGES.emptyNoneHint
          : BILLING_PRICING_MESSAGES.emptyNoMatchHint
      "
      :icon="!hasActiveFilter ? Coins : Search"
    >
      <template #actions>
        <button
          v-if="hasActiveFilter"
          type="button"
          class="flex h-8 items-center gap-1.5 rounded-control border border-border bg-card px-3 text-xs font-medium text-foreground transition-colors duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] hover:bg-muted cursor-pointer"
          @click="clearFilters"
        >
          <Xmark class="size-3.5" />
          <span>{{ BILLING_PRICING_MESSAGES.clearFilters }}</span>
        </button>
      </template>
    </AdminEmptyState>

    <!-- 4. 分 namespace 定价表（Image / Other，ai-search 暂归 Other，对齐任务监控页事件表） -->
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

    <!-- 5. 统一服务设置弹窗（定价与模型池） -->
    <BillingPriceDialog
      v-model:visible="priceDialogVisible"
      :item="selectedItem"
      :pending="publishPending"
      :pool-entries="poolEditEntries"
      :pool-catalog-models="poolCatalogModels"
      :pool-catalog-loading="poolCatalogLoading"
      :pool-save-pending="poolSavePending"
      :pool-save-stale="poolSaveStale"
      :pool-save-disabled="!poolEditReady"
      :pool-catalog-error="poolCatalogError"
      @save="saveSettings"
      @reload-pool="reloadPool"
      @reload-catalog="loadPoolCatalog"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { Coins, Search, Xmark } from '@iconoir/vue'
import { AdminEmptyState } from '../../../shared'
import { AdminRefreshButton } from '../../../shared'
import { AdminSpinner } from '../../../shared'
import { AdminToolbarSearch } from '../../../shared'
import { getAdminServiceOrder, adminDashboardRootClass } from '../../../shared'
import { formatAdminDisplayError, useAdminToast } from '../../../shared'
import BillingPricingTable from './BillingPricingTable.vue'
import BillingPriceDialog from './BillingPriceDialog.vue'
import { useConfirm } from '@/core/composables/useConfirmDialog'
import { confirmSettingsSave } from '../../../shared'
import { createAdminBillableCatalogResource } from '../../session/pricing/billableCatalogResource'
import { createAdminModelPoolResource } from '../../session/pricing/modelPoolResource'
import { useBillingPricingFeedback } from '../../session/pricing/useBillingPricingFeedback'
import { serviceOfSku } from '../../core/pricing/skuFeature'
import {
  PRICING_NAMESPACE_TABLES,
  PRICING_OTHER_TABLE,
  pricingNamespaceRank,
  pricingTableKeyOfSku,
} from '../../core/pricing/groups'
import type {
  AdminBillableItemCard,
  AdminModelPool,
  AdminModelPoolEntry,
  AdminPoolCatalogModel,
  PricingVariant,
} from '../../core/pricing/billableCatalogTypes'
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
const savedPoolEntries = ref<AdminModelPoolEntry[]>([])
const poolEditReady = ref(false)
const poolCatalogModels = ref<AdminPoolCatalogModel[]>([])
const poolCatalogLoading = ref(false)
const poolCatalogError = ref('')
const poolSavePending = ref(false)
const poolSaveStale = ref(false)

const isLoading = ref(true)
const selectedItem = ref<AdminBillableItemCard | null>(null)
const priceDialogVisible = ref(false)
const publishPending = ref(false)
const availabilityPending = reactive<Record<string, boolean>>({})
const deletePending = reactive<Record<string, boolean>>({})
const toast = useAdminToast()
const confirm = useConfirm()
const pricingFeedback = useBillingPricingFeedback()

const searchKeyword = ref('')

const hasActiveFilter = computed(() => Boolean(searchKeyword.value.trim()))

function clearFilters() {
  searchKeyword.value = ''
}

// 过滤后的列表
const filteredItems = computed(() => {
  const query = searchKeyword.value.trim().toLowerCase()

  return sortedItems.value.filter((item) => {
    if (!query) return true

    if (item.displayName.toLowerCase().includes(query)) return true
    if (item.sku.toLowerCase().includes(query)) return true

    return false
  })
})

function compareCatalogCards(left: AdminBillableItemCard, right: AdminBillableItemCard) {
  const namespaceDifference = pricingNamespaceRank(left.sku) - pricingNamespaceRank(right.sku)
  if (namespaceDifference !== 0) return namespaceDifference
  const serviceDifference =
    getAdminServiceOrder(serviceOfSku(left.sku) ?? '') -
    getAdminServiceOrder(serviceOfSku(right.sku) ?? '')
  if (serviceDifference !== 0) return serviceDifference
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

async function loadCatalog() {
  isLoading.value = true
  try {
    items.value = await resource.list()
    await loadModelPools()
  } catch (error) {
    toast.error(formatAdminDisplayError(error, BILLING_PRICING_MESSAGES.loadFailedTitle))
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
    // 目录行内错误文案已删除：失败只走 toast，面板重载按钮保留为恢复入口。
    toast.error(poolCatalogError.value)
  } finally {
    poolCatalogLoading.value = false
  }
}

async function reloadPool() {
  const item = poolEditItem.value
  if (!item) return
  poolEditReady.value = false
  poolSavePending.value = true
  poolSaveStale.value = false
  try {
    const pool = await poolResource.get(item.sku)
    poolBySku.value[item.sku] = pool
    poolFailed[item.sku] = false
    poolEditVersion.value = pool.version
    poolEditEntries.value = clonePoolEntries(pool.entries)
    savedPoolEntries.value = clonePoolEntries(pool.entries)
    poolEditReady.value = true
  } catch (error) {
    poolEditVersion.value = 0
    poolEditEntries.value = []
    savedPoolEntries.value = []
    poolEditReady.value = false
    poolSaveStale.value = true
    // 冲突恢复入口（重新加载按钮）保留在面板内，失败明细只走 toast。
    toast.error(formatAdminDisplayError(error, BILLING_PRICING_MESSAGES.poolSaveFailed))
  } finally {
    poolSavePending.value = false
  }
}

function applyPoolReplaceSuccess(item: AdminBillableItemCard, saved: AdminModelPool) {
  poolBySku.value[item.sku] = saved
  poolFailed[item.sku] = false
  poolEditVersion.value = saved.version
  poolEditEntries.value = clonePoolEntries(saved.entries)
  savedPoolEntries.value = clonePoolEntries(saved.entries)
}

function clonePoolEntries(entries: AdminModelPoolEntry[]): AdminModelPoolEntry[] {
  return entries.map((entry) => ({
    modelId: entry.modelId,
    enabled: entry.enabled,
    resolutions: [...entry.resolutions],
  }))
}

function handlePoolSaveError(error: unknown) {
  // 409 冲突额外保留 stale 恢复入口（重新加载按钮）。
  toast.error(formatAdminDisplayError(error, BILLING_PRICING_MESSAGES.poolSaveFailed))
  poolSaveStale.value = error instanceof Error && (error as { code?: unknown }).code === 'conflict'
}

async function savePool(entries: AdminModelPoolEntry[]) {
  const item = poolEditItem.value
  if (!item || poolSavePending.value || !poolEditReady.value) return

  poolSavePending.value = true
  poolSaveStale.value = false
  try {
    const saved = await poolResource.replace(item.sku, { version: poolEditVersion.value, entries })
    applyPoolReplaceSuccess(item, saved)
    priceDialogVisible.value = false
    toast.success(BILLING_PRICING_MESSAGES.poolSaved)
  } catch (error) {
    handlePoolSaveError(error)
  } finally {
    poolSavePending.value = false
  }
}

onMounted(loadCatalog)

async function openPriceDialog(item: AdminBillableItemCard) {
  selectedItem.value = item
  priceDialogVisible.value = true
  if (serviceOfSku(item.sku)) {
    poolEditItem.value = item
    poolEditReady.value = false
    poolSaveStale.value = false
    await reloadPool()
    if (poolCatalogModels.value.length === 0 && !poolCatalogLoading.value) {
      await loadPoolCatalog()
    }
  }
}

async function confirmArchivedDelete(item: AdminBillableItemCard) {
  if (item.status !== 'archived' || deletePending[item.sku]) return
  const isConfirmed = await confirm.confirm({
    title: BILLING_PRICING_MESSAGES.deleteTitle,
    message: BILLING_PRICING_MESSAGES.deleteMessage(item.displayName),
    confirmLabel: BILLING_PRICING_MESSAGES.deleteConfirm,
    intent: 'danger',
  })
  if (!isConfirmed) return

  await deleteArchived(item)
}

async function deleteArchived(item: AdminBillableItemCard) {
  if (deletePending[item.sku]) return
  deletePending[item.sku] = true
  try {
    await resource.deleteArchived(item.sku)
    items.value = items.value.filter((candidate) => candidate.sku !== item.sku)
    toast.success(BILLING_PRICING_MESSAGES.deleted)
  } catch (error) {
    toast.error(formatAdminDisplayError(error, BILLING_PRICING_MESSAGES.deleteFailed))
  } finally {
    deletePending[item.sku] = false
  }
}

function applyPricePublishSuccess(
  item: AdminBillableItemCard,
  published: AdminBillableItemCard['currentPrices'],
) {
  item.currentPrices = published
  const latest = Object.values(published).sort((a, b) => b.createdAt.localeCompare(a.createdAt))[0]
  if (latest) item.updatedAt = latest.createdAt
  if (item.status === 'unpriced') item.status = 'active'
}

async function publishPrice(prices: Record<PricingVariant, number | undefined>) {
  const item = selectedItem.value
  if (!item || publishPending.value) return
  const isConfirmed = await confirmSettingsSave(confirm, {
    title: BILLING_PRICING_MESSAGES.publishTitle,
    message: BILLING_PRICING_MESSAGES.publishMessage(item.displayName),
    confirmLabel: BILLING_PRICING_MESSAGES.publishConfirm,
  })
  if (!isConfirmed) return
  publishPending.value = true
  try {
    const published = await resource.publishPrices(item.sku, prices)
    applyPricePublishSuccess(item, published)
    priceDialogVisible.value = false
    toast.success(BILLING_PRICING_MESSAGES.published)
  } catch (error) {
    // 单定价发布失败只走 toast（对话框保持打开可重试），与组合保存一致。
    toast.error(formatAdminDisplayError(error, BILLING_PRICING_MESSAGES.publishFailed))
  } finally {
    publishPending.value = false
  }
}

function isPricingChanged(
  item: AdminBillableItemCard,
  newPrices: Record<PricingVariant, number | undefined>,
): boolean {
  if (item.status === 'unpriced') return true
  for (const variant of item.pricingVariants) {
    const oldVal = item.currentPrices[variant]?.unitCredits
    const newVal = newPrices[variant]
    if (oldVal !== newVal) return true
  }
  return false
}

function isPoolChanged(
  originalEntries: AdminModelPoolEntry[],
  newEntries?: AdminModelPoolEntry[],
): boolean {
  if (!newEntries) return false
  if (originalEntries.length !== newEntries.length) return true
  for (let i = 0; i < originalEntries.length; i++) {
    const orig = originalEntries[i]!
    const next = newEntries[i]!
    if (orig.modelId !== next.modelId || orig.enabled !== next.enabled) return true
    if (orig.resolutions.length !== next.resolutions.length) return true
    const origSet = new Set(orig.resolutions)
    if (next.resolutions.some((r) => !origSet.has(r))) return true
  }
  return false
}

async function saveCombinedSettings(
  item: AdminBillableItemCard,
  prices: Record<PricingVariant, number | undefined>,
  entries: AdminModelPoolEntry[],
) {
  const isConfirmed = await confirmSettingsSave(confirm, {
    title: BILLING_PRICING_MESSAGES.saveAllTitle,
    message: BILLING_PRICING_MESSAGES.saveAllMessage(item.displayName),
    confirmLabel: BILLING_PRICING_MESSAGES.saveAllConfirm,
  })
  if (!isConfirmed) return

  publishPending.value = true
  poolSavePending.value = true
  poolSaveStale.value = false

  const pricingChanged = isPricingChanged(item, prices)
  const poolChanged = isPoolChanged(savedPoolEntries.value, entries)
  let failed = false
  let pricingFailed = false
  let poolFailed = false
  let poolConflicted = false
  let pricingDetail = ''
  let poolDetail = ''

  try {
    if (pricingChanged) {
      publishPending.value = true
      try {
        const published = await resource.publishPrices(item.sku, prices)
        applyPricePublishSuccess(item, published)
      } catch (error) {
        pricingFailed = true
        pricingDetail = formatAdminDisplayError(error, BILLING_PRICING_MESSAGES.publishFailed)
        failed = true
      } finally {
        publishPending.value = false
      }
    }

    if (poolChanged) {
      poolSavePending.value = true
      try {
        const saved = await poolResource.replace(item.sku, {
          version: poolEditVersion.value,
          entries,
        })
        applyPoolReplaceSuccess(item, saved)
      } catch (error) {
        poolFailed = true
        poolDetail = formatAdminDisplayError(error, BILLING_PRICING_MESSAGES.poolSaveFailed)
        poolConflicted = error instanceof Error && (error as { code?: unknown }).code === 'conflict'
        failed = true
      } finally {
        poolSavePending.value = false
      }
    }
  } finally {
    publishPending.value = false
    poolSavePending.value = false
  }

  if (failed) {
    // 组合保存部分失败仅 toast 命名失败部分，不写 inline，避免双通知重复；
    // 单定价/单池路径同样只走 toast。409（pool conflict）是例外：toast 已带
    // 明细，同时置 poolSaveStale 让对话框内出现“重新加载”入口，否则用户无处刷新。
    poolSaveStale.value = poolConflicted
    const failedTabs: string[] = []
    const details: string[] = []
    if (pricingFailed) {
      failedTabs.push(BILLING_PRICING_MESSAGES.tabPricing)
      if (pricingDetail) details.push(pricingDetail)
    }
    if (poolFailed) {
      failedTabs.push(BILLING_PRICING_MESSAGES.tabModelPool)
      if (poolDetail) details.push(poolDetail)
    }
    pricingFeedback.combinedSaveFailed(failedTabs, details)
  } else {
    priceDialogVisible.value = false
    toast.success(BILLING_PRICING_MESSAGES.saveAllSuccess)
  }
}

async function saveSettings(payload: {
  prices: Record<PricingVariant, number | undefined>
  poolEntries?: AdminModelPoolEntry[] | undefined
}) {
  const item = selectedItem.value
  if (!item || publishPending.value || poolSavePending.value) return

  const pricingChanged = isPricingChanged(item, payload.prices)
  const poolChanged =
    Boolean(serviceOfSku(item.sku)) &&
    Boolean(payload.poolEntries) &&
    isPoolChanged(savedPoolEntries.value, payload.poolEntries)

  if (pricingChanged && poolChanged && payload.poolEntries) {
    await saveCombinedSettings(item, payload.prices, payload.poolEntries)
  } else if (poolChanged && payload.poolEntries) {
    await savePool(payload.poolEntries)
  } else {
    await publishPrice(payload.prices)
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
