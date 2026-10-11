<template>
  <AccountPane :title="t('account.tabs.topUp')">
    <ReturnStatus
      v-if="paymentReturn"
      :pending="returnPending"
      :error="returnError"
      :source="paymentReturn.source"
      :status="returnStatus"
      @retry="retryReturn()"
    />
    <PendingOrders
      :items="pendingOrders"
      :pending="pendingOrdersPending"
      :error="pendingOrdersError"
      :action-pending="pendingOrderActionPending"
      :action-target="pendingOrderActionTarget"
      :action-error="pendingOrderActionError"
      @retry="retryPendingOrders()"
      @continue="continuePending"
      @abandon="abandonPending"
    />
    <PurchaseCard
      :options="purchaseOptions"
      :pending="optionsPending"
      :error="optionsError"
      :checkout-pending="checkoutPending"
      :checkout-error="checkoutError"
      @retry="retryOptions()"
      @checkout="checkout"
    />
    <HistoryTable
      :items="historyItems"
      :initial-pending="historyInitialPending"
      :initial-error="historyInitialError"
      :load-more-error="historyLoadMoreError"
      :has-next-page="historyHasNextPage"
      :fetching-next-page="historyFetchingNextPage"
      @retry="retryHistory()"
      @load-more="loadMoreHistory()"
    />
  </AccountPane>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { toRef } from 'vue'
import type { PaymentReturnContext } from '../../core'
import { useRecharge } from '../../session/payments/useRecharge'
import AccountPane from '../shared/AccountPane.vue'
import ReturnStatus from './ReturnStatus.vue'
import HistoryTable from './HistoryTable.vue'
import PendingOrders from './PendingOrders.vue'
import PurchaseCard from './PurchaseCard.vue'

const props = defineProps<{ active: boolean; paymentReturn: PaymentReturnContext | null }>()
const { t } = useI18n()
const {
  purchaseOptions,
  optionsPending,
  optionsError,
  retryOptions,
  createCheckout,
  checkoutPending,
  checkoutError,
  returnStatus,
  returnPending,
  returnError,
  retryReturn,
  historyItems,
  historyInitialPending,
  historyInitialError,
  historyLoadMoreError,
  historyHasNextPage,
  historyFetchingNextPage,
  retryHistory,
  loadMoreHistory,
  pendingOrders,
  pendingOrdersPending,
  pendingOrdersError,
  retryPendingOrders,
  continuePendingOrder,
  abandonPendingOrder,
  pendingOrderActionPending,
  pendingOrderActionTarget,
  pendingOrderActionError,
} = useRecharge(toRef(props, 'active'), toRef(props, 'paymentReturn'))

async function checkout(ideaQuantity: number) {
  try {
    await createCheckout(ideaQuantity)
  } catch {
    // useMutation retains the failure for the visible, retryable form error state.
  }
}

async function continuePending(publicToken: string) {
  try {
    await continuePendingOrder(publicToken)
  } catch {
    // The pending-order card retains a visible error and retry action.
  }
}

async function abandonPending(publicToken: string) {
  try {
    await abandonPendingOrder(publicToken)
  } catch {
    // The pending-order card retains a visible error and retry action.
  }
}
</script>
