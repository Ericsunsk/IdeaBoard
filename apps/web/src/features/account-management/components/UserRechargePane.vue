<template>
  <AccountManagementPane title="充值积分">
    <UserPaymentReturnStatus
      v-if="paymentReturn"
      :pending="returnPending"
      :error="returnError"
      :source="paymentReturn.source"
      :status="returnStatus"
      @retry="retryReturn()"
    />
    <UserRechargePurchaseCard
      :options="purchaseOptions"
      :pending="optionsPending"
      :error="optionsError"
      :checkout-pending="checkoutPending"
      :checkout-error="checkoutError"
      @retry="retryOptions()"
      @checkout="checkout"
    />
    <UserRechargeHistoryTable
      :items="historyItems"
      :initial-pending="historyInitialPending"
      :initial-error="historyInitialError"
      :load-more-error="historyLoadMoreError"
      :has-next-page="historyHasNextPage"
      :fetching-next-page="historyFetchingNextPage"
      @retry="retryHistory()"
      @load-more="loadMoreHistory()"
    />
  </AccountManagementPane>
</template>

<script setup lang="ts">
import { toRef } from 'vue'
import type { PaymentReturnContext } from '../paymentReturnRoute'
import { useUserRecharge } from '../useUserRecharge'
import AccountManagementPane from './AccountManagementPane.vue'
import UserPaymentReturnStatus from './UserPaymentReturnStatus.vue'
import UserRechargeHistoryTable from './UserRechargeHistoryTable.vue'
import UserRechargePurchaseCard from './UserRechargePurchaseCard.vue'

const props = defineProps<{ active: boolean; paymentReturn: PaymentReturnContext | null }>()
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
} = useUserRecharge(toRef(props, 'active'), toRef(props, 'paymentReturn'))

async function checkout(ideaQuantity: number) {
  try {
    await createCheckout(ideaQuantity)
  } catch {
    // useMutation retains the failure for the visible, retryable form error state.
  }
}
</script>
