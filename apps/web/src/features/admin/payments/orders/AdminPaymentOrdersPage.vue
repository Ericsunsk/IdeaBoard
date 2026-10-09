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
        :input-label="PAYMENT_ORDERS_MESSAGES.orderSearchLabel"
        placeholder="IBP-YYYYMMDD-…"
        class="w-full sm:w-64"
        @enter="applySearch"
        @clear="clearOrderNumberSearch"
      />
      <AdminToolbarSearch
        v-model="userSearch"
        :input-label="PAYMENT_ORDERS_MESSAGES.userSearchLabel"
        :placeholder="PAYMENT_ORDERS_MESSAGES.userSearchPlaceholder"
        class="w-full sm:w-56"
        @enter="applySearch"
        @clear="clearUserSearch"
      />
    </template>

    <template #toolbar-end>
      <AdminFilterPopover
        :active="hasFilters"
        :filter-label="PAYMENT_ORDERS_MESSAGES.filterLabel"
        :title="PAYMENT_ORDERS_MESSAGES.filterTitle"
        @reset="resetFilters"
      >
        <div class="flex min-w-0 flex-col gap-3.5">
          <label
            class="flex flex-col gap-1.5 text-admin-meta font-admin-strong text-muted-foreground"
          >
            <span>{{ PAYMENT_ORDERS_MESSAGES.statusLabel }}</span>
            <AdminSelect v-model="statusFilter" :options="paymentOrderStatusOptions" size="small" />
          </label>
          <label
            class="flex items-center justify-between gap-4 text-admin-meta font-admin-strong text-muted-foreground"
          >
            <span>{{ PAYMENT_ORDERS_MESSAGES.anomalyOnly }}</span>
            <AdminSwitch
              v-model="anomalyOnly"
              class="shrink-0"
              :aria-label="PAYMENT_ORDERS_MESSAGES.anomalyOnly"
            />
          </label>
        </div>
      </AdminFilterPopover>

      <AdminButton
        variant="ghost"
        :class="adminFramedIconButtonClass"
        :aria-label="PAYMENT_ORDERS_MESSAGES.refresh"
        :loading="loading"
        :disabled="loading"
        @click="refreshOrders"
      >
        <Refresh class="size-4 text-muted-foreground" />
      </AdminButton>
    </template>

    <template #alerts>
      <AdminAlert v-if="initialError" severity="error" class="m-4">
        <div class="flex flex-wrap items-center justify-between gap-3">
          <span>{{ PAYMENT_ORDERS_MESSAGES.initialError }}</span>
          <ActionButton variant="outline" :disabled="loading" @click="refreshOrders">
            {{ PAYMENT_ORDERS_MESSAGES.retry }}
          </ActionButton>
        </div>
      </AdminAlert>

      <AdminAlert v-else-if="backgroundError" severity="warn" class="mx-4 mt-4">
        <div class="flex flex-wrap items-center justify-between gap-3">
          <span>{{ PAYMENT_ORDERS_MESSAGES.backgroundError }}</span>
          <ActionButton variant="outline" :disabled="loading" @click="refreshOrders">
            {{ PAYMENT_ORDERS_MESSAGES.retry }}
          </ActionButton>
        </div>
      </AdminAlert>
    </template>

    <AdminPaymentOrdersTable
      v-if="!initialError"
      :orders="orders"
      :loading="loading"
      :empty-title="emptyTitle"
      @open-detail="openDetail"
    />

    <template #footer-label>{{ PAYMENT_ORDERS_MESSAGES.footerLabel(totalLabel) }}</template>
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
import { Refresh } from '@iconoir/vue'
import AdminCollectionPageShell from '../../shared/components/AdminCollectionPageShell.vue'
import AdminAlert from '../../shared/components/AdminAlert.vue'
import AdminButton from '../../shared/components/AdminButton.vue'
import ActionButton from '../../../../shared/components/ActionButton.vue'
import AdminSelect from '../../shared/components/AdminSelect.vue'
import AdminSwitch from '../../shared/components/AdminSwitch.vue'
import AdminFilterPopover from '../../shared/components/AdminFilterPopover.vue'
import AdminToolbarSearch from '../../shared/components/AdminToolbarSearch.vue'
import { useAdminConfirm } from '../../shared/composables/useAdminConfirm'
import { requireDestructiveReason } from '../../shared/lib/confirmSettingsSave'
import { formatAdminDisplayError } from '../../shared/lib/toast'
import { usePaymentOrdersFeedback } from '../useStripeSettingsFeedback'
import { adminFramedIconButtonClass } from '../../shared/lib/uiClasses'
import { createUniqueId } from '../../../../shared/utils/ids'
import { formatLocaleNumber } from '../../../../shared/i18n/formatters'
import PaymentOrderDetailDialog from './PaymentOrderDetailDialog.vue'
import AdminPaymentOrdersTable from './AdminPaymentOrdersTable.vue'
import {
  ORDER_STATUS_FILTER_ALL,
  paymentOrderStatusOptions,
  type PaymentOrderStatusFilter,
} from './paymentOrderPresentation'
import { createAdminPaymentOrderResource } from './paymentOrderResource'
import { PAYMENT_ORDERS_MESSAGES } from './messages'
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

const totalLabel = computed(() =>
  PAYMENT_ORDERS_MESSAGES.totalUnit(formatLocaleNumber(total.value)),
)
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
const emptyTitle = computed(() =>
  hasCriteria.value ? PAYMENT_ORDERS_MESSAGES.emptyNoMatch : PAYMENT_ORDERS_MESSAGES.emptyNone,
)

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
  // Control refunds require an operator reason (audit); a cancelled dialog
  // aborts silently.
  const confirmed = await adminConfirm.confirm({
    title: PAYMENT_ORDERS_MESSAGES.refundTitle,
    message: PAYMENT_ORDERS_MESSAGES.refundMessage(
      publicOrderNumber,
      formatLocaleNumber(ideaQuantity),
    ),
    confirmLabel: PAYMENT_ORDERS_MESSAGES.refundConfirm,
    intent: 'danger',
    reason: { required: true },
  })
  if (!confirmed) return
  const operatorReason = requireDestructiveReason(adminConfirm)
  if (!operatorReason) return
  await executeRefund(publicOrderNumber, ideaQuantity, operatorReason)
}

async function executeRefund(
  publicOrderNumber: string,
  ideaQuantity: number,
  operatorReason: string,
) {
  const key = `${publicOrderNumber}:${ideaQuantity}`
  let idempotencyKey = refundIdempotencyKeys.get(key)
  if (!idempotencyKey) {
    idempotencyKey = createUniqueId()
    refundIdempotencyKeys.set(key, idempotencyKey)
  }
  refundLoading.value = true
  refundError.value = ''
  try {
    detail.value = await resource.refund(publicOrderNumber, {
      ideaQuantity,
      idempotencyKey,
      operatorReason,
    })
    refundIdempotencyKeys.delete(key)
    feedback.refundSubmitted()
    refreshOrders()
  } catch (error) {
    refundError.value = formatAdminDisplayError(error, PAYMENT_ORDERS_MESSAGES.refundFailed)
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
</script>
