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
            input-label="搜索定价功能"
            placeholder="搜索功能名称、SKU 或映射模型..."
          />

          <AdminOptionFilter
            v-model="statusFilter"
            :options="statusFilterOptions"
            label="筛选功能状态"
          />
        </div>

        <!-- 右侧：刷新同步按钮 -->
        <div class="flex shrink-0 items-center gap-2">
          <AdminRefreshButton
            :loading="isLoading"
            :disabled="modelsLoading"
            aria-label="刷新功能定价"
            @click="loadCatalog"
          />
        </div>
      </div>

      <!-- 次级统计提示条：共 X 个计费功能 -->
      <div v-if="!isLoading" class="flex items-center justify-between pt-1">
        <div class="flex items-center gap-2 text-sm text-admin-text-2">
          <span class="font-medium text-admin-text-1"
            >共 {{ filteredItems.length }} 个计费功能</span
          >
          <span v-if="hasActiveFilter" class="text-xs text-admin-text-3">
            (共 {{ sortedItems.length }} 个)
          </span>
        </div>
      </div>
    </div>

    <!-- 2. 加载中状态 (AdminSpinner 自带 role="progressbar") -->
    <div
      v-if="isLoading && items.length === 0"
      class="flex min-h-[260px] items-center justify-center rounded-xl border border-admin-border-soft bg-white text-admin-text-3"
      aria-label="加载功能定价中"
    >
      <AdminSpinner class="size-9 text-admin-accent" />
    </div>

    <!-- 3. 错误告警状态 -->
    <div v-else-if="errorMessage" :class="adminEmptyStateClass" role="alert">
      <div class="text-admin-title font-semibold text-admin-title-color">功能定价加载失败</div>
      <div class="mt-2 text-admin-meta text-admin-subtitle-color">{{ errorMessage }}</div>
      <AdminButton class="mt-4" variant="outline" :disabled="isLoading" @click="loadCatalog">
        <Refresh class="size-4" />
        重新加载
      </AdminButton>
    </div>

    <!-- 4. 空状态提示 (无数据或过滤未命中) -->
    <div
      v-else-if="filteredItems.length === 0"
      class="flex min-h-[320px] flex-col items-center justify-center rounded-xl border border-dashed border-admin-border-soft bg-white p-10 text-center"
    >
      <div
        class="mb-3.5 flex size-12 items-center justify-center rounded-2xl bg-admin-surface-muted text-admin-text-3"
      >
        <Coins v-if="!hasActiveFilter" class="size-6" />
        <Search v-else class="size-6" />
      </div>
      <h3 class="text-base font-semibold text-admin-title-color">
        {{ !hasActiveFilter ? '暂无可定价功能' : '未找到匹配的计费功能' }}
      </h3>
      <p class="mt-1 max-w-sm text-xs text-admin-text-3">
        {{
          !hasActiveFilter
            ? '当前没有已注册的可计费功能 SKU。'
            : '未找到符合当前搜索或筛选条件的功能。请尝试调整关键词或清空筛选。'
        }}
      </p>
      <button
        v-if="hasActiveFilter"
        type="button"
        class="mt-4 flex h-8 items-center gap-1.5 rounded-lg border border-admin-border-soft bg-white px-3 text-xs font-medium text-admin-text-1 transition-colors hover:bg-admin-surface-muted cursor-pointer"
        @click="clearFilters"
      >
        <Xmark class="size-3.5" />
        <span>清空筛选条件</span>
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
        @availability="setAvailability"
        @publish="openPriceDialog"
        @delete="confirmArchivedDelete"
      >
        <template #model="{ item }">
          <span
            class="truncate font-mono text-xs"
            :class="
              modelValueFor(item.sku) ? 'text-admin-text-1' : 'text-admin-text-3'
            "
          >
            {{ modelDisplayFor(item.sku) }}
          </span>
        </template>
      </BillingPricingTable>
    </div>

    <!-- 6. 价格发布调整弹窗 -->
    <BillingPriceDialog
      v-model:visible="priceDialogVisible"
      :item="selectedItem"
      :pending="publishPending"
      :error-message="publishError"
      :model-value="
        selectedItem ? modelValueFor(selectedItem.sku) : undefined
      "
      :model-pending="
        selectedItem ? modelPendingFor(selectedItem.sku) : undefined
      "
      :model-error="
        selectedItem ? modelErrorFor(selectedItem.sku) : undefined
      "
      :available-models="availableModels"
      @submit="publishPrice"
      @save-model="onDialogSaveModel"
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
import { createAdminBillableCatalogResource } from './billableCatalogResource'
import { usePricingModelAdapter } from './usePricingModelAdapter'
import {
  PRICING_NAMESPACE_TABLES,
  PRICING_OTHER_TABLE,
  findPricingMappingCollisions,
  pricingMappingFeature,
  pricingNamespaceRank,
  pricingTableKeyOfSku,
} from './groups'
import type { AdminBillableItemCard, PricingVariant } from './billableCatalogTypes'

const resource = createAdminBillableCatalogResource()
const items = ref<AdminBillableItemCard[]>([])
const sortedItems = computed(() => [...items.value].sort(compareCatalogCards))

// 模型映射域出口
const modelAdapter = usePricingModelAdapter()
const {
  availableModels,
  modelsLoading,
  loadModels,
  saveModelForItem,
  modelValueFor,
  modelPendingFor,
  modelErrorFor,
} = modelAdapter

// Model 列文案：不可映射（如已归档 SKU）显示 —，与价格列不适用态一致；
// 可映射未配置才显示未配置，与 Dialog 无映射行（只设价格）对齐。
function modelDisplayFor(sku: string): string {
  if (pricingMappingFeature(sku) === null) return '—'
  return modelValueFor(sku) ?? '未配置'
}

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
  { label: '全部状态', value: '' },
  { label: '生效中', value: 'active' },
  { label: '未定价', value: 'unpriced' },
  { label: '已停用', value: 'disabled' },
  { label: '已归档', value: 'archived' },
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

    const mappedModel = modelValueFor(item.sku)
    if (mappedModel?.toLowerCase().includes(query)) return true

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
    // 裸 key 碰撞兜底：同 bare feature 被多 namespace 认领时保存 A 即改 B 的显示，
    // 当前目录无碰撞，仅防新增同名 feature，需人工确认后端 key 空间。
    const collisions = findPricingMappingCollisions(items.value.map((item) => item.sku))
    if (collisions.length > 0) {
      console.warn('[pricing] mapping feature collision across namespaces', collisions)
      toast.warning(
        `模型映射存在同名冲突：${collisions.map((c) => c.feature).join('、')}，保存会影响多行，请确认后端 key 空间`,
      )
    }
    // 目录成功才拉模型：失败页不再叠加模型错误 toast；刷新按钮看两者。
    await loadModels()
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

function onDialogSaveModel(model: string) {
  if (selectedItem.value) saveModelForItem(selectedItem.value, model)
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
