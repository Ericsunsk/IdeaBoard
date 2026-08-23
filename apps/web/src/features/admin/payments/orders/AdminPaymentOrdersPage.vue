<template>
  <AdminTablePanel viewport-height content-class="min-h-0 flex-1 overflow-hidden px-6 pb-6">
    <template #toolbar>
      <AdminResourceToolbar>
        <template #search>
          <div class="flex min-w-0 flex-wrap gap-2">
            <AdminToolbarSearch
              v-model="orderNumberSearch"
              input-label="搜索公开订单号"
              placeholder="IBP-YYYYMMDD-…"
              @enter="applySearch"
              @clear="clearOrderNumberSearch"
            />
            <AdminToolbarSearch
              v-model="userSearch"
              input-label="搜索支付用户"
              placeholder="用户 ID 或用户名"
              @enter="applySearch"
              @clear="clearUserSearch"
            />
          </div>
        </template>

        <template #metrics="{ metricClass }">
          <span :class="metricClass">
            <span class="h-2.5 w-2.5 rounded-full bg-admin-success"></span>
            <span>筛选结果 {{ totalLabel }}</span>
          </span>
        </template>

        <template #filters>
          <AdminFilterPopover
            :active="hasFilters"
            filter-label="筛选支付订单"
            title="订单状态与异常"
            @reset="resetFilters"
          >
            <div class="flex min-w-0 flex-col gap-4">
              <label class="flex flex-col gap-1.5 text-admin-meta font-[650] text-admin-text-2">
                <span>订单状态</span>
                <Select
                  v-model="statusFilter"
                  :options="paymentOrderStatusOptions"
                  option-label="label"
                  option-value="value"
                  placeholder="全部状态"
                  size="small"
                  show-clear
                  fluid
                />
              </label>
              <label
                class="flex items-center justify-between gap-4 text-admin-meta font-[650] text-admin-text-2"
              >
                <span>仅看 24 小时异常</span>
                <ToggleSwitch
                  v-model="anomalyOnly"
                  class="shrink-0"
                  aria-label="仅看 24 小时异常"
                />
              </label>
            </div>
          </AdminFilterPopover>
        </template>

        <template #actions="{ iconButtonClass }">
          <Button
            text
            :class="iconButtonClass"
            :disabled="loading"
            aria-label="刷新支付订单"
            @click="refreshOrders"
          >
            <template #icon><Refresh class="size-4 text-admin-text-2" /></template>
          </Button>
        </template>

        <template #pager>
          <AdminPagerControls
            :current-page="pagination.currentPage.value"
            :total-pages="pagination.knownPageCount.value"
            :prev-disabled="!pagination.canRetreat.value"
            :next-disabled="!pagination.canAdvance.value"
            @prev="previousPage"
            @next="nextPage"
          />
        </template>
      </AdminResourceToolbar>
    </template>

    <Message v-if="initialError" severity="error" :closable="false" class="m-5">
      <div class="flex flex-wrap items-center justify-between gap-3">
        <span>支付订单加载失败，请重试。</span>
        <Button
          label="重试"
          severity="secondary"
          size="small"
          outlined
          :class="adminActionButtonClass"
          :disabled="loading"
          @click="refreshOrders"
        />
      </div>
    </Message>

    <Message v-else-if="backgroundError" severity="warn" :closable="false" class="mx-5 mt-5">
      <div class="flex flex-wrap items-center justify-between gap-3">
        <span>刷新支付订单失败，正在显示上次成功的数据。</span>
        <Button
          label="重试"
          severity="secondary"
          size="small"
          outlined
          :class="adminActionButtonClass"
          :disabled="loading"
          @click="refreshOrders"
        />
      </div>
    </Message>

    <AdminEmptyState
      v-if="showEmptyState"
      :title="emptyTitle"
    />

    <DataTableFrame
      v-else-if="!initialError"
      :value="orders"
      :loading="loading"
      data-key="publicOrderNumber"
      scrollable
      scroll-height="flex"
      frame-class="h-full min-h-0"
      table-aria-label="支付订单列表"
      scroll-aria-label="支付订单列表横向滚动区域"
      :table-style="{ minWidth: '1260px' }"
    >
      <Column field="createdAt" header="订单时间" :style="{ width: '170px' }">
        <template #body="{ data }">
          <time :datetime="data.createdAt" class="text-admin-meta text-admin-text-3">
            {{ formatPaymentDateTime(data.createdAt) }}
          </time>
        </template>
      </Column>

      <Column field="publicOrderNumber" header="公开订单号" :style="{ minWidth: '215px' }">
        <template #body="{ data }">
          <Button
            link
            class="!p-0 font-mono !text-admin-content !font-bold"
            :label="data.publicOrderNumber"
            :aria-label="`查看订单 ${data.publicOrderNumber}`"
            @click="openDetail(data.publicOrderNumber)"
          />
        </template>
      </Column>

      <Column header="用户" :style="{ width: '150px' }">
        <template #body="{ data }">
          <div class="min-w-0">
            <div class="truncate text-admin-body font-bold text-admin-text-1">
              {{ data.user.displayName }}
            </div>
          </div>
        </template>
      </Column>

      <Column field="ideaQuantity" header="Idea 数量" :style="{ width: '120px' }">
        <template #body="{ data }">{{ ideaFormatter.format(data.ideaQuantity) }}</template>
      </Column>

      <Column header="原始实付" :style="{ width: '145px' }">
        <template #body="{ data }">
          {{ formatPaymentMoney(data.paidAmountMinor, data.paidCurrency) }}
        </template>
      </Column>

      <Column field="stripeStatus" header="Stripe 状态" :style="{ width: '135px' }">
        <template #body="{ data }">
          <Tag
            :value="getStripeStatusLabel(data.stripeStatus)"
            :severity="stripeStatusSeverity(data.stripeStatus)"
            rounded
          />
        </template>
      </Column>

      <Column field="creditStatus" header="本地到账" :style="{ width: '125px' }">
        <template #body="{ data }">
          <Tag
            :value="getCreditStatusLabel(data.creditStatus)"
            :severity="creditStatusSeverity(data.creditStatus)"
            rounded
          />
        </template>
      </Column>

      <Column header="异常" :style="{ width: '115px' }">
        <template #body="{ data }">
          <Tag v-if="data.anomalousAt" value="24h 未收敛" severity="danger" rounded />
          <span v-else class="text-admin-meta text-admin-text-3">—</span>
        </template>
      </Column>

      <Column header="详情" :style="{ width: '72px' }">
        <template #body="{ data }">
          <Button
            text
            size="small"
            severity="secondary"
            :class="adminFramedIconButtonClass"
            :aria-label="`查看订单 ${data.publicOrderNumber}`"
            @click="openDetail(data.publicOrderNumber)"
          >
            <template #icon><Eye class="size-4 text-admin-text-2" /></template>
          </Button>
        </template>
      </Column>
    </DataTableFrame>
  </AdminTablePanel>

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
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { Eye, Refresh } from '@iconoir/vue'
import Button from 'primevue/button'
import Column from 'primevue/column'
import Message from 'primevue/message'
import Select from 'primevue/select'
import Tag from 'primevue/tag'
import ToggleSwitch from 'primevue/toggleswitch'
import DataTableFrame from '../../../../shared/components/DataTableFrame.vue'
import AdminEmptyState from '../../shared/components/AdminEmptyState.vue'
import AdminFilterPopover from '../../shared/components/AdminFilterPopover.vue'
import AdminPagerControls from '../../shared/components/AdminPagerControls.vue'
import AdminResourceToolbar from '../../shared/components/AdminResourceToolbar.vue'
import AdminTablePanel from '../../shared/components/AdminTablePanel.vue'
import AdminToolbarSearch from '../../shared/components/AdminToolbarSearch.vue'
import { useAdminCollectionState } from '../../shared/composables/useAdminCollectionState'
import { useAdminCursorPagination } from '../../shared/composables/useAdminCursorPagination'
import { useAdminConfirm } from '../../shared/composables/useAdminConfirm'
import { formatAdminDisplayError, useAdminToast } from '../../shared/lib/adminToast'
import {
  adminActionButtonClass,
  adminFramedIconButtonClass,
} from '../../shared/lib/adminUiClasses'
import PaymentOrderDetailDialog from './PaymentOrderDetailDialog.vue'
import {
  formatPaymentDateTime,
  formatPaymentMoney,
  getCreditStatusLabel,
  getStripeStatusLabel,
  paymentOrderStatusOptions,
} from './paymentOrderPresentation'
import { createAdminPaymentOrderResource } from './paymentOrderResource'
import type { AdminPaymentOrderDetail, AdminPaymentOrderStatus } from './paymentOrderTypes'

const resource = createAdminPaymentOrderResource()
const orderNumberSearch = ref('')
const appliedOrderNumber = ref('')
const userSearch = ref('')
const appliedUser = ref('')
const statusFilter = ref<AdminPaymentOrderStatus>()
const anomalyOnly = ref(false)
let pagination: ReturnType<typeof useAdminCursorPagination<string>>
const detailVisible = ref(false)
const selectedOrderNumber = ref('')
const detail = ref<AdminPaymentOrderDetail | null>(null)
const detailLoading = ref(false)
const detailError = ref(false)
const refundLoading = ref(false)
const refundError = ref('')
const ideaFormatter = new Intl.NumberFormat('zh-CN')
const adminConfirm = useAdminConfirm()
const toast = useAdminToast()
const refundIdempotencyKeys = new Map<string, string>()
let detailRequestEpoch = 0
let activeDetailRequest: AbortController | undefined

const collection = useAdminCollectionState({
  load: (signal) =>
    resource.search({
      orderNumber: appliedOrderNumber.value,
      user: appliedUser.value,
      status: statusFilter.value,
      anomalyOnly: anomalyOnly.value,
      cursor: pagination.cursor.value,
      signal,
    }),
  isEmpty: (page) => page.orders.length === 0,
})
const { data: orderPage, loading, initialError, backgroundError, hasLoadedResult } = collection
pagination = useAdminCursorPagination<string>({ pending: loading })
const orders = computed(() => orderPage.value?.orders ?? [])
const total = computed(() => orderPage.value?.total ?? 0)

const totalLabel = computed(() => `${ideaFormatter.format(total.value)} 笔`)
const hasFilters = computed(() => Boolean(statusFilter.value || anomalyOnly.value))
const hasCriteria = computed(() =>
  Boolean(
    appliedOrderNumber.value.trim() ||
    appliedUser.value.trim() ||
    statusFilter.value ||
    anomalyOnly.value,
  ),
)
const showEmptyState = computed(
  () => hasLoadedResult.value && !loading.value && !initialError.value && collection.isEmpty.value,
)
const emptyTitle = computed(() => (hasCriteria.value ? '未找到匹配的支付订单' : '暂无支付订单'))

onMounted(() => {
  void loadOrders()
})

onBeforeUnmount(() => {
  collection.cancel()
  activeDetailRequest?.abort()
})

watch([statusFilter, anomalyOnly], () => {
  resetPaginationAndLoad()
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

async function loadOrders({ preserveData = false }: { preserveData?: boolean } = {}) {
  const page = await collection.reload({ preserveData })
  if (page) pagination.setNextCursor(page.nextCursor)
}

function applySearch() {
  appliedOrderNumber.value = orderNumberSearch.value
  appliedUser.value = userSearch.value
  resetPaginationAndLoad()
}

function clearOrderNumberSearch() {
  appliedOrderNumber.value = ''
  resetPaginationAndLoad()
}

function clearUserSearch() {
  appliedUser.value = ''
  resetPaginationAndLoad()
}

function resetFilters() {
  statusFilter.value = undefined
  anomalyOnly.value = false
}

function refreshOrders() {
  void loadOrders({ preserveData: true })
}

function resetPaginationAndLoad() {
  pagination.reset()
  void loadOrders()
}

function previousPage() {
  if (!pagination.retreat()) return
  void loadOrders()
}

function nextPage() {
  if (!pagination.advance()) return
  void loadOrders()
}

function openDetail(publicOrderNumber: string) {
  selectedOrderNumber.value = publicOrderNumber
  detailVisible.value = true
  void loadDetail()
}

function retryDetail() {
  void loadDetail()
}

function requestRefund(ideaQuantity: number) {
  const publicOrderNumber = selectedOrderNumber.value
  if (!publicOrderNumber || refundLoading.value) return
  adminConfirm.request({
    title: '确认退款',
    message: `将为订单 ${publicOrderNumber} 发起 ${ideaFormatter.format(ideaQuantity)} Ideas 退款。退款提交后会先预留余额，是否继续？`,
    confirmLabel: '发起退款',
    intent: 'danger',
    onConfirm: () => executeRefund(publicOrderNumber, ideaQuantity),
  })
}

async function executeRefund(publicOrderNumber: string, ideaQuantity: number) {
  const key = `${publicOrderNumber}:${ideaQuantity}`
  let idempotencyKey = refundIdempotencyKeys.get(key)
  if (!idempotencyKey) {
    idempotencyKey = crypto.randomUUID()
    refundIdempotencyKeys.set(key, idempotencyKey)
  }
  refundLoading.value = true
  refundError.value = ''
  try {
    detail.value = await resource.refund(publicOrderNumber, { ideaQuantity, idempotencyKey })
    refundIdempotencyKeys.delete(key)
    toast.success('退款已提交，等待 Stripe 最终处理')
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
