<template>
  <AdminCollectionPageShell
    :current-page="pagination.currentPage.value"
    :total-pages="pagination.knownPageCount.value"
    :prev-disabled="!pagination.canRetreat.value"
    :next-disabled="!pagination.canAdvance.value"
    @prev="previousPage"
    @next="nextPage"
  >
    <template #toolbar-start>
      <AdminToolbarSearch
        v-model="orderNumberSearch"
        input-label="搜索公开订单号"
        placeholder="IBP-YYYYMMDD-…"
        class="w-full sm:w-64"
        @enter="applySearch"
        @clear="clearOrderNumberSearch"
      />
      <AdminToolbarSearch
        v-model="userSearch"
        input-label="搜索支付用户"
        placeholder="用户 ID 或用户名"
        class="w-full sm:w-56"
        @enter="applySearch"
        @clear="clearUserSearch"
      />
    </template>

    <template #toolbar-end>
      <AdminFilterPopover
        :active="hasFilters"
        filter-label="筛选支付订单"
        title="订单状态与异常"
        @reset="resetFilters"
      >
        <div class="flex min-w-0 flex-col gap-3.5">
          <label class="flex flex-col gap-1.5 text-admin-meta font-[650] text-admin-text-2">
            <span>订单状态</span>
            <AdminSelect v-model="statusFilter" :options="paymentOrderStatusOptions" size="small" />
          </label>
          <label
            class="flex items-center justify-between gap-4 text-admin-meta font-[650] text-admin-text-2"
          >
            <span>仅看 24 小时异常</span>
            <AdminSwitch v-model="anomalyOnly" class="shrink-0" aria-label="仅看 24 小时异常" />
          </label>
        </div>
      </AdminFilterPopover>

      <AdminButton
        variant="ghost"
        :class="adminFramedIconButtonClass"
        aria-label="刷新支付订单"
        :loading="loading"
        :disabled="loading"
        @click="refreshOrders"
      >
        <Refresh class="size-4 text-admin-text-2" />
      </AdminButton>
    </template>

    <template #alerts>
      <AdminAlert v-if="initialError" severity="error" class="m-4">
        <div class="flex flex-wrap items-center justify-between gap-3">
          <span>支付订单加载失败，请重试。</span>
          <AdminButton
            variant="outline"
            size="small"
            :class="adminActionButtonClass"
            :disabled="loading"
            @click="refreshOrders"
          >
            重试
          </AdminButton>
        </div>
      </AdminAlert>

      <AdminAlert v-else-if="backgroundError" severity="warn" class="mx-4 mt-4">
        <div class="flex flex-wrap items-center justify-between gap-3">
          <span>刷新支付订单失败，正在显示上次成功的数据。</span>
          <AdminButton
            variant="outline"
            size="small"
            :class="adminActionButtonClass"
            :disabled="loading"
            @click="refreshOrders"
          >
            重试
          </AdminButton>
        </div>
      </AdminAlert>
    </template>

    <AdminTable
      v-if="!initialError"
      :value="orders"
      :loading="loading"
      data-key="publicOrderNumber"
      scrollable
      :frame-class="adminTableFrameClass"
      table-aria-label="支付订单列表"
      scroll-aria-label="支付订单列表横向滚动区域"
      :table-style="adminTableStyle"
      :class="adminTableClass"
    >
      <template #empty>
        <AdminEmptyState :title="emptyTitle" />
      </template>

      <!-- 订单时间列：等宽日期 -->
      <AdminColumn field="createdAt" header="订单时间" :style="{ width: '160px' }">
        <template #body="{ data }">
          <time
            :datetime="data.createdAt"
            class="whitespace-nowrap font-mono text-xs text-admin-text-3"
          >
            {{ formatPaymentDateTime(data.createdAt) }}
          </time>
        </template>
      </AdminColumn>

      <!-- 公开订单号列：等宽可点击链接 -->
      <AdminColumn field="publicOrderNumber" header="公开订单号" :style="{ minWidth: '220px' }">
        <template #body="{ data }">
          <button
            type="button"
            class="font-mono text-xs font-semibold text-admin-text-1 hover:text-admin-accent cursor-pointer transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-admin-accent/20 rounded"
            :aria-label="`查看订单 ${data.publicOrderNumber}`"
            @click="openDetail(data.publicOrderNumber)"
          >
            {{ data.publicOrderNumber }}
          </button>
        </template>
      </AdminColumn>

      <!-- 用户列：姓名 -->
      <AdminColumn header="用户" :style="{ width: '160px' }">
        <template #body="{ data }">
          <div class="truncate font-medium text-admin-text-1">
            {{ data.user.displayName }}
          </div>
        </template>
      </AdminColumn>

      <!-- Idea 数量列：等宽数字 -->
      <AdminColumn field="ideaQuantity" header="Idea 数量" :style="{ width: '120px' }">
        <template #body="{ data }">
          <span class="font-mono text-xs font-semibold text-admin-text-1">
            {{ ideaFormatter.format(data.ideaQuantity) }}
          </span>
        </template>
      </AdminColumn>

      <!-- 原始实付列：等宽金额 -->
      <AdminColumn header="原始实付" :style="{ width: '130px' }">
        <template #body="{ data }">
          <span class="font-mono text-xs font-semibold text-admin-text-1">
            {{ formatPaymentMoney(data.paidAmountMinor, data.paidCurrency) }}
          </span>
        </template>
      </AdminColumn>

      <!-- Stripe 状态列：圆角徽标 -->
      <AdminColumn field="stripeStatus" header="Stripe 状态" :style="{ width: '120px' }">
        <template #body="{ data }">
          <AdminBadge
            :value="getStripeStatusLabel(data.stripeStatus)"
            :severity="stripeStatusSeverity(data.stripeStatus)"
            rounded
          />
        </template>
      </AdminColumn>

      <!-- 本地到账列：圆角徽标 -->
      <AdminColumn field="creditStatus" header="本地到账" :style="{ width: '120px' }">
        <template #body="{ data }">
          <AdminBadge
            :value="getCreditStatusLabel(data.creditStatus)"
            :severity="creditStatusSeverity(data.creditStatus)"
            rounded
          />
        </template>
      </AdminColumn>

      <!-- 异常列：Badge 或横线 -->
      <AdminColumn header="异常" :style="{ width: '110px' }">
        <template #body="{ data }">
          <AdminBadge v-if="data.anomalousAt" value="24h 未收敛" severity="danger" rounded />
          <span v-else class="text-xs text-admin-text-3">—</span>
        </template>
      </AdminColumn>

      <!-- 详情操作列 (1:1 对齐用户管理页操作列样式) -->
      <AdminColumn header="详情" :style="{ width: '64px' }">
        <template #body="{ data }">
          <div class="flex items-center justify-start">
            <button
              type="button"
              :class="adminTableRowActionButtonClass"
              :aria-label="`查看订单 ${data.publicOrderNumber}`"
              @click="openDetail(data.publicOrderNumber)"
            >
              <Eye class="size-4" />
            </button>
          </div>
        </template>
      </AdminColumn>
    </AdminTable>

    <template #footer-label>筛选结果 {{ totalLabel }}</template>
  </AdminCollectionPageShell>

  <PaymentOrderDetailDialog
    v-model:visible="detailVisible"
    :detail="detail"
    :loading="detailLoading"
    :error="detailError"
    :refunding="refundLoading"
    :refund-error="refundError"
    @retry="retryDetail"
    @refund="requestRefund"
  />
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useAdminCursorCollection } from '../../shared/composables/useAdminCursorCollection'
import { Eye, Refresh } from '@iconoir/vue'
import AdminCollectionPageShell from '../../shared/components/AdminCollectionPageShell.vue'
import AdminColumn from '../../shared/components/AdminColumn.vue'
import AdminAlert from '../../shared/components/AdminAlert.vue'
import AdminBadge from '../../shared/components/AdminBadge.vue'
import AdminButton from '../../shared/components/AdminButton.vue'
import AdminSelect from '../../shared/components/AdminSelect.vue'
import AdminSwitch from '../../shared/components/AdminSwitch.vue'
import AdminTable from '../../shared/components/AdminTable.vue'
import AdminEmptyState from '../../shared/components/AdminEmptyState.vue'
import AdminFilterPopover from '../../shared/components/AdminFilterPopover.vue'
import AdminToolbarSearch from '../../shared/components/AdminToolbarSearch.vue'
import { useAdminConfirm } from '../../shared/composables/useAdminConfirm'
import { formatAdminDisplayError } from '../../shared/lib/toast'
import { usePaymentOrdersFeedback } from '../useStripeSettingsFeedback'
import {
  adminActionButtonClass,
  adminFramedIconButtonClass,
  adminTableClass,
  adminTableFrameClass,
  adminTableRowActionButtonClass,
  adminTableStyle,
} from '../../shared/lib/uiClasses'
import { createUniqueId } from '../../../../shared/utils/ids'
import PaymentOrderDetailDialog from './PaymentOrderDetailDialog.vue'
import {
  formatPaymentDateTime,
  formatPaymentMoney,
  getCreditStatusLabel,
  getStripeStatusLabel,
  ORDER_STATUS_FILTER_ALL,
  paymentOrderStatusOptions,
  type PaymentOrderStatusFilter,
} from './paymentOrderPresentation'
import { createAdminPaymentOrderResource } from './paymentOrderResource'
import type { AdminPaymentOrderDetail, SearchAdminPaymentOrdersInput } from './paymentOrderTypes'

const resource = createAdminPaymentOrderResource()
const orderNumberSearch = ref('')
const appliedOrderNumber = ref('')
const userSearch = ref('')
const appliedUser = ref('')
// 筛选哨兵对齐用户管理页：'all' 表示不过滤，落 query key 前再归一为 undefined。
const statusFilter = ref<PaymentOrderStatusFilter>(ORDER_STATUS_FILTER_ALL)
const anomalyOnly = ref(false)
const detailVisible = ref(false)
const selectedOrderNumber = ref('')
const detail = ref<AdminPaymentOrderDetail | null>(null)
const detailLoading = ref(false)
const detailError = ref(false)
const refundLoading = ref(false)
const refundError = ref('')
const ideaFormatter = new Intl.NumberFormat('zh-CN')
const adminConfirm = useAdminConfirm()
const feedback = usePaymentOrdersFeedback()
const refundIdempotencyKeys = new Map<string, string>()
let detailRequestEpoch = 0
let activeDetailRequest: AbortController | undefined

// 游标集合生命周期收敛于 useAdminCursorCollection：筛选/分页状态喂给 query key，
// 翻页与刷新只改状态。
const ORDER_LIST_QUERY_ROOT = ['admin', 'payments', 'orders'] as const
const ORDER_LIST_CRITERIA_INDEX = ORDER_LIST_QUERY_ROOT.length
const {
  data: orderPage,
  loading,
  initialError,
  backgroundError,
  refresh: refreshOrderList,
  pagination,
  previousPage,
  nextPage,
} = useAdminCursorCollection({
  buildKey: (cursor) =>
    [
      ...ORDER_LIST_QUERY_ROOT,
      {
        orderNumber: appliedOrderNumber.value,
        user: appliedUser.value,
        status: statusFilter.value === ORDER_STATUS_FILTER_ALL ? undefined : statusFilter.value,
        anomalyOnly: anomalyOnly.value,
        cursor,
      },
    ] as const,
  queryFn: ({ queryKey, signal }) =>
    resource.search({
      ...(queryKey[ORDER_LIST_CRITERIA_INDEX] as Omit<
        SearchAdminPaymentOrdersInput,
        'signal' | 'limit'
      >),
      signal,
    }),
  getNextCursor: (page) => page.nextCursor,
})
const orders = computed(() => orderPage.value?.orders ?? [])
const total = computed(() => orderPage.value?.total ?? 0)

const totalLabel = computed(() => `${ideaFormatter.format(total.value)} 笔`)
const hasFilters = computed(
  () => statusFilter.value !== ORDER_STATUS_FILTER_ALL || anomalyOnly.value,
)
const hasCriteria = computed(() =>
  Boolean(
    appliedOrderNumber.value.trim() ||
    appliedUser.value.trim() ||
    statusFilter.value !== ORDER_STATUS_FILTER_ALL ||
    anomalyOnly.value,
  ),
)
const emptyTitle = computed(() => (hasCriteria.value ? '未找到匹配的支付订单' : '暂无支付订单'))

// The query fetches on mount and cancels on unmount by itself; only the
// detail request (still hand-rolled, out of pilot scope) needs teardown here.
onBeforeUnmount(() => {
  activeDetailRequest?.abort()
})

watch([statusFilter, anomalyOnly], () => {
  resetPaginationAndReload()
})

watch(detailVisible, (visible) => {
  if (visible) return
  detailRequestEpoch += 1
  activeDetailRequest?.abort()
  selectedOrderNumber.value = ''
  detail.value = null
  detailLoading.value = false
  detailError.value = false
  refundLoading.value = false
  refundError.value = ''
})

function refreshOrders() {
  // Same-key refetch: stale rows stay mounted with the table loading state,
  // matching the previous background-refresh behavior.
  void refreshOrderList()
}

function resetPaginationAndReload() {
  // Cursor/filter changes feed the query key and refetch by themselves.
  pagination.reset()
}

function applySearch() {
  appliedOrderNumber.value = orderNumberSearch.value
  appliedUser.value = userSearch.value
  resetPaginationAndReload()
}

function clearOrderNumberSearch() {
  appliedOrderNumber.value = ''
  resetPaginationAndReload()
}

function clearUserSearch() {
  appliedUser.value = ''
  resetPaginationAndReload()
}

function resetFilters() {
  statusFilter.value = ORDER_STATUS_FILTER_ALL
  anomalyOnly.value = false
}

function openDetail(publicOrderNumber: string) {
  selectedOrderNumber.value = publicOrderNumber
  detailVisible.value = true
  void loadDetail()
}

function retryDetail() {
  void loadDetail()
}

async function requestRefund(ideaQuantity: number) {
  const publicOrderNumber = selectedOrderNumber.value
  if (!publicOrderNumber || refundLoading.value) return
  const confirmed = await adminConfirm.confirm({
    title: '确认退款',
    message: `将为订单 ${publicOrderNumber} 发起 ${ideaFormatter.format(ideaQuantity)} Ideas 退款。退款提交后会先预留余额，是否继续？`,
    confirmLabel: '发起退款',
    intent: 'danger',
  })
  if (!confirmed) return
  await executeRefund(publicOrderNumber, ideaQuantity)
}

async function executeRefund(publicOrderNumber: string, ideaQuantity: number) {
  const key = `${publicOrderNumber}:${ideaQuantity}`
  let idempotencyKey = refundIdempotencyKeys.get(key)
  if (!idempotencyKey) {
    idempotencyKey = createUniqueId()
    refundIdempotencyKeys.set(key, idempotencyKey)
  }
  refundLoading.value = true
  refundError.value = ''
  try {
    detail.value = await resource.refund(publicOrderNumber, { ideaQuantity, idempotencyKey })
    refundIdempotencyKeys.delete(key)
    feedback.refundSubmitted()
    refreshOrders()
  } catch (error) {
    refundError.value = formatAdminDisplayError(error, '发起退款失败，请稍后重试')
  } finally {
    refundLoading.value = false
  }
}

async function loadDetail() {
  const publicOrderNumber = selectedOrderNumber.value
  if (!publicOrderNumber) return
  const epoch = ++detailRequestEpoch
  activeDetailRequest?.abort()
  const controller = new AbortController()
  activeDetailRequest = controller
  detail.value = null
  detailLoading.value = true
  detailError.value = false
  try {
    const result = await resource.get(publicOrderNumber, controller.signal)
    if (
      controller.signal.aborted ||
      epoch !== detailRequestEpoch ||
      selectedOrderNumber.value !== publicOrderNumber
    ) {
      return
    }
    detail.value = result
  } catch {
    if (!controller.signal.aborted && epoch === detailRequestEpoch) detailError.value = true
  } finally {
    if (!controller.signal.aborted && epoch === detailRequestEpoch) detailLoading.value = false
  }
}

function stripeStatusSeverity(status: string) {
  if (status === 'paid') return 'success'
  if (status === 'payment_failed') return 'danger'
  if (status === 'open' || status === 'async_payment_pending') return 'warn'
  return 'secondary'
}

function creditStatusSeverity(status: string) {
  if (status === 'credited') return 'success'
  if (status === 'pending') return 'warn'
  return 'secondary'
}
</script>
