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
      @retry="retryPendingOrders()"
      @continue="continuePending"
      @abandon="abandonPending"
    />
    <PurchaseCard
      :options="purchaseOptions"
      :pending="optionsPending"
      :checkout-pending="checkoutPending"
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
import { useAppToast } from '../../../../core/composables/useAppToast'
import { useFailureToast } from '../../../../core/composables/useFailureToast'
import { userFacingErrorCopy } from '../../../../core/errors/errorCatalog'
import type { PaymentReturnContext } from '../../core/return'
import { useRecharge } from '../../session/payments/useRecharge'
import { ACCOUNT_GUIDANCE_COPY_KEYS } from '../../session/workspaceBilling'
import AccountPane from '../shared/AccountPane.vue'
import ReturnStatus from './ReturnStatus.vue'
import HistoryTable from './HistoryTable.vue'
import PendingOrders from './PendingOrders.vue'
import PurchaseCard from './PurchaseCard.vue'

const props = defineProps<{ isActive: boolean; paymentReturn: PaymentReturnContext | null }>()
const { t } = useI18n()
const toast = useAppToast()
const {
  purchaseOptions,
  optionsPending,
  optionsError,
  retryOptions,
  createCheckout,
  checkoutPending,
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
} = useRecharge(toRef(props, 'isActive'), toRef(props, 'paymentReturn'))

function errorCopy(error: unknown, fallbackKey: string): string {
  return userFacingErrorCopy(error, t(fallbackKey), ACCOUNT_GUIDANCE_COPY_KEYS)
}

// Read failures use toast feedback; pagination retains its inline hint.
useFailureToast(optionsError, (error) => errorCopy(error, 'account.errors.optionsLoadFailed'))
useFailureToast(pendingOrdersError, (error) =>
  errorCopy(error, 'account.errors.pendingOrdersLoadFailed'),
)
useFailureToast(historyInitialError, (error) =>
  errorCopy(error, 'account.errors.historyLoadFailed'),
)
useFailureToast(returnError, (error) => errorCopy(error, 'account.errors.orderStatusLoadFailed'))

async function checkout(ideaQuantity: number) {
  try {
    await createCheckout(ideaQuantity)
  } catch (error) {
    toast.error(errorCopy(error, 'account.errors.checkoutCreateFailed'))
  }
}

async function continuePending(publicToken: string) {
  try {
    await continuePendingOrder(publicToken)
  } catch (error) {
    toast.error(errorCopy(error, 'account.errors.continuePaymentFailed'))
  }
}

async function abandonPending(publicToken: string) {
  try {
    await abandonPendingOrder(publicToken)
  } catch (error) {
    toast.error(errorCopy(error, 'account.errors.abandonOrderFailed'))
  }
}
</script>
