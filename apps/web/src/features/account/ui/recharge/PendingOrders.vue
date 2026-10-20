<template>
  <section
    v-if="pending || error || items.length"
    class="grid gap-3"
    aria-labelledby="pending-payment-orders-heading"
  >
    <h3 id="pending-payment-orders-heading" class="m-0 text-lg font-bold text-foreground">
      {{ t('account.topUp.pendingOrdersTitle') }}
    </h3>

    <div
      v-if="pending"
      class="grid gap-2 rounded-panel border border-border bg-card p-4 shadow-raised"
      role="status"
      :aria-label="t('account.topUp.pendingOrdersLoading')"
    >
      <Skeleton height="1.25rem" width="42%" />
      <Skeleton height="1rem" width="68%" />
    </div>

    <LoadErrorPanel
      v-else-if="error"
      :message="t('account.errors.pendingOrdersLoadFailed')"
      :detail="errorMessage(error)"
      :retry-label="t('account.reload')"
      retry-variant="ghost"
      @retry="emit('retry')"
    />

    <div v-else class="grid gap-2">
      <article
        v-for="item in items"
        :key="item.public_token"
        :data-testid="`pending-payment-order-${item.public_token}`"
        class="rounded-panel border border-border bg-card p-3.5 shadow-raised sm:p-4"
      >
        <div class="flex min-w-0 flex-col items-start justify-between gap-4 sm:flex-row">
          <div class="min-w-0">
            <p class="m-0 font-mono text-lg font-bold text-foreground">
              {{ formatIdeaAmount(item.idea_quantity) }}
            </p>
            <p class="mt-1 mb-0 text-sm font-semibold" :class="statusTone(item.status)">
              {{ statusText(item) }}
            </p>
            <p class="mt-1 mb-0 text-admin-body text-muted-foreground">
              {{ expiryText(item) }}
            </p>
          </div>

          <div v-if="item.status === 'checkout_open'" class="grid w-full shrink-0 gap-2 sm:w-36">
            <Button
              :is-loading="isActionPending(item.public_token, 'continue')"
              :is-disabled="actionPending"
              :class="[accountPrimaryActionButtonClass, 'w-full']"
              @click="emit('continue', item.public_token)"
              >{{ t('account.topUp.continuePayment') }}</Button
            >
            <Button
              variant="outline"
              size="sm"
              class="w-full"
              :is-loading="isActionPending(item.public_token, 'abandon')"
              :is-disabled="actionPending"
              @click="emit('abandon', item.public_token)"
              >{{ t('account.topUp.abandonOrder') }}</Button
            >
          </div>
        </div>

        <div
          v-if="actionError && actionTarget?.publicToken === item.public_token"
          class="mt-3 flex flex-wrap items-center gap-x-3 gap-y-1 rounded-control border border-destructive/25 bg-muted px-3 py-2"
          role="alert"
        >
          <span class="text-sm font-semibold text-destructive">{{
            errorMessage(actionError)
          }}</span>
          <Button
            variant="ghost"
            size="sm"
            class="!p-0"
            :is-disabled="actionPending"
            @click="retryAction(item.public_token)"
            >{{
              actionTarget?.kind === 'continue'
                ? t('account.topUp.retryContinuePayment')
                : t('account.topUp.retryAbandonOrder')
            }}</Button
          >
        </div>
      </article>
    </div>
  </section>
</template>

<script setup lang="ts">
import { useNow } from '@vueuse/core'
import { useI18n } from 'vue-i18n'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { formatIdeaAmount } from '../../../../core/i18n/userIdeaCurrency'
import LoadErrorPanel from '../../../../core/components/LoadErrorPanel.vue'
import { formatAccountExpiryDateTime } from '../../session/formatters'
import { userFacingErrorCopy } from '../../../../core/errors/errorCatalog'
import { ACCOUNT_GUIDANCE_COPY_KEYS } from '../../session/workspaceBilling'
import { accountPrimaryActionButtonClass } from '../shared/ui'
import type { PendingPaymentOrder } from '../../session/payments/resource'

type PendingOrderAction = { publicToken: string; kind: 'continue' | 'abandon' }

const props = defineProps<{
  items: PendingPaymentOrder[]
  pending: boolean
  error: unknown
  actionPending: boolean
  actionTarget: PendingOrderAction | null
  actionError: unknown
}>()
const emit = defineEmits<{
  retry: []
  continue: [publicToken: string]
  abandon: [publicToken: string]
}>()

const { t } = useI18n()
// 5s ticker for expiry/delayed labels — `useNow` owns the interval lifecycle.
const now = useNow({ interval: 5_000 })

function isActionPending(publicToken: string, kind: PendingOrderAction['kind']) {
  return (
    props.actionPending &&
    props.actionTarget?.publicToken === publicToken &&
    props.actionTarget.kind === kind
  )
}

function retryAction(publicToken: string) {
  if (props.actionTarget?.kind === 'continue') {
    emit('continue', publicToken)
    return
  }
  emit('abandon', publicToken)
}

function statusText(item: PendingPaymentOrder) {
  switch (item.status) {
    case 'checkout_open':
      return t('account.topUp.statusCheckoutOpen')
    case 'async_payment_pending':
      return t('account.topUp.statusAsyncPending')
    case 'paid_pending_credit':
      return isCreditProcessingDelayed(item.updated_at)
        ? t('account.topUp.statusCreditingDelayed')
        : t('account.topUp.statusCrediting')
    case 'expired':
      return t('account.topUp.statusExpired')
    case 'cancelled':
      return t('account.topUp.statusCancelled')
    case 'payment_failed':
      return t('account.topUp.statusPaymentFailed')
    case 'credited':
      return t('account.topUp.statusCredited')
  }
}

function statusTone(status: PendingPaymentOrder['status']) {
  if (status === 'credited') return 'text-success'
  if (status === 'expired' || status === 'cancelled' || status === 'payment_failed') {
    return 'text-muted-foreground'
  }
  return 'text-primary'
}

function isCreditProcessingDelayed(updatedAt: string) {
  const updatedAtMs = new Date(updatedAt).getTime()
  return Number.isFinite(updatedAtMs) && now.value.getTime() - updatedAtMs > 60_000
}

function expiryText(item: PendingPaymentOrder) {
  const date = new Date(item.expires_at)
  if (Number.isNaN(date.getTime())) return t('account.topUp.expiryUnknown')
  const formatted = formatAccountExpiryDateTime(date)
  return item.status === 'expired'
    ? t('account.topUp.expiredAt', { time: formatted })
    : t('account.topUp.expiresAt', { time: formatted })
}

function errorMessage(error: unknown) {
  return userFacingErrorCopy(error, t('account.errors.networkRetry'), ACCOUNT_GUIDANCE_COPY_KEYS)
}
</script>
