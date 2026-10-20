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
import { useAppToast } from '../../../../core/composables/useAppToast'
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
} = useRecharge(toRef(props, 'isActive'), toRef(props, 'paymentReturn'))

async function checkout(ideaQuantity: number) {
  try {
    await createCheckout(ideaQuantity)
  } catch (error) {
    // Dual: useMutation retains the failure for the visible, retryable form
    // error state (PurchaseCard inline alert as position anchor); toast
    // surfaces it transiently for users who miss the inline bar.
    toast.error(
      userFacingErrorCopy(
        error,
        t('account.errors.checkoutCreateFailed'),
        ACCOUNT_GUIDANCE_COPY_KEYS,
      ),
    )
  }
}

async function continuePending(publicToken: string) {
  try {
    await continuePendingOrder(publicToken)
  } catch (error) {
    // Dual: the pending-order card retains a visible error and retry action;
    // toast mirrors it transiently.
    toast.error(
      userFacingErrorCopy(
        error,
        t('account.errors.continuePaymentFailed'),
        ACCOUNT_GUIDANCE_COPY_KEYS,
      ),
    )
  }
}

async function abandonPending(publicToken: string) {
  try {
    await abandonPendingOrder(publicToken)
  } catch (error) {
    // Dual: the pending-order card retains a visible error and retry action;
    // toast mirrors it transiently.
    toast.error(
      userFacingErrorCopy(
        error,
        t('account.errors.abandonOrderFailed'),
        ACCOUNT_GUIDANCE_COPY_KEYS,
      ),
    )
  }
}
</script>
