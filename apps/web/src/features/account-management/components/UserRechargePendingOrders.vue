<template>
  <section
    v-if="pending || error || items.length"
    class="grid gap-3"
    aria-labelledby="pending-payment-orders-heading"
  >
    <h3 id="pending-payment-orders-heading" class="m-0 text-lg font-bold text-ib-text">
      {{ t('account.topUp.pendingOrdersTitle') }}
    </h3>

    <div
      v-if="pending"
      class="grid gap-2 rounded-[var(--radius-ib-panel)] border border-ib-border-soft bg-ib-surface p-4 shadow-[var(--shadow-ib-control)]"
      role="status"
      :aria-label="t('account.topUp.pendingOrdersLoading')"
    >
      <Skeleton height="1.25rem" width="42%" />
      <Skeleton height="1rem" width="68%" />
    </div>

    <div
      v-else-if="error"
      class="rounded-[var(--radius-ib-panel)] border border-ib-danger/25 bg-ib-surface p-4 shadow-[var(--shadow-ib-control)]"
      role="alert"
    >
      <p class="m-0 font-semibold text-ib-danger">
        {{ t('account.errors.pendingOrdersLoadFailed') }}
      </p>
      <p class="mt-1 mb-0 text-sm text-ib-text-muted">{{ errorMessage(error) }}</p>
      <Button
        :label="t('account.reload')"
        severity="secondary"
        size="small"
        text
        class="mt-2"
        @click="emit('retry')"
      />
    </div>

    <div v-else class="grid gap-2">
      <article
        v-for="item in items"
        :key="item.public_token"
        :data-testid="`pending-payment-order-${item.public_token}`"
        class="rounded-[var(--radius-ib-panel)] border border-ib-border-soft bg-ib-surface p-4 shadow-[var(--shadow-ib-control)] max-[640px]:p-3.5"
      >
        <div class="flex min-w-0 items-start justify-between gap-4 max-[640px]:flex-col">
          <div class="min-w-0">
            <p class="m-0 font-mono text-lg font-bold text-ib-text">
              {{ formatIdeaAmount(item.idea_quantity) }}
            </p>
            <p class="mt-1 mb-0 text-sm font-semibold" :class="statusTone(item.status)">
              {{ statusText(item) }}
            </p>
            <p class="mt-1 mb-0 text-[13px] text-ib-text-muted">
              {{ expiryText(item) }}
            </p>
          </div>

          <div
            v-if="item.status === 'checkout_open'"
            class="grid w-36 shrink-0 gap-2 max-[640px]:w-full"
          >
            <Button
              unstyled
              :label="t('account.topUp.continuePayment')"
              size="small"
              :loading="isActionPending(item.public_token, 'continue')"
              :disabled="actionPending"
              :class="[accountPrimaryActionButtonClass, 'w-full']"
              :pt="accountButtonPt"
              @click="emit('continue', item.public_token)"
            />
            <Button
              :label="t('account.topUp.abandonOrder')"
              severity="secondary"
              size="small"
              outlined
              class="w-full"
              :loading="isActionPending(item.public_token, 'abandon')"
              :disabled="actionPending"
              @click="emit('abandon', item.public_token)"
            />
          </div>
        </div>

        <div
          v-if="actionError && actionTarget?.publicToken === item.public_token"
          class="mt-3 flex flex-wrap items-center gap-x-3 gap-y-1 rounded-[var(--radius-ib-control)] border border-ib-danger/25 bg-ib-surface-muted px-3 py-2"
          role="alert"
        >
          <span class="text-sm font-semibold text-ib-danger">{{ errorMessage(actionError) }}</span>
          <Button
            :label="
              actionTarget.kind === 'continue'
                ? t('account.topUp.retryContinuePayment')
                : t('account.topUp.retryAbandonOrder')
            "
            severity="secondary"
            size="small"
            text
            class="!p-0"
            :disabled="actionPending"
            @click="retryAction(item.public_token)"
          />
        </div>
      </article>
    </div>
  </section>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Button from 'primevue/button'
import Skeleton from 'primevue/skeleton'
import { formatIdeaAmount } from '../../../shared/i18n/userIdeaCurrency'
import { formatLocaleDate } from '../../../shared/i18n/formatters'
import { userFacingErrorCopy } from '../../../shared/errors/errorCatalog'
import { accountButtonPt, accountPrimaryActionButtonClass } from './accountManagementUi'
import type { UserPendingPaymentOrder } from '../userPaymentsResource'

type PendingOrderAction = { publicToken: string; kind: 'continue' | 'abandon' }

const props = defineProps<{
  items: UserPendingPaymentOrder[]
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
const now = ref(Date.now())
let refreshTimer: ReturnType<typeof setInterval> | undefined

onMounted(() => {
  refreshTimer = setInterval(() => {
    now.value = Date.now()
  }, 5_000)
})
onBeforeUnmount(() => {
  if (refreshTimer) clearInterval(refreshTimer)
})

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

function statusText(item: UserPendingPaymentOrder) {
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

function statusTone(status: UserPendingPaymentOrder['status']) {
  if (status === 'credited') return 'text-ib-success'
  if (status === 'expired' || status === 'cancelled' || status === 'payment_failed') {
    return 'text-ib-text-muted'
  }
  return 'text-ib-creative-accent-pressed'
}

function isCreditProcessingDelayed(updatedAt: string) {
  const updatedAtMs = new Date(updatedAt).getTime()
  return Number.isFinite(updatedAtMs) && now.value - updatedAtMs > 60_000
}

function expiryText(item: UserPendingPaymentOrder) {
  const date = new Date(item.expires_at)
  if (Number.isNaN(date.getTime())) return t('account.topUp.expiryUnknown')
  const formatted = formatLocaleDate(date, {
    dateStyle: 'medium',
    timeStyle: 'short',
    hour12: false,
  })
  return item.status === 'expired'
    ? t('account.topUp.expiredAt', { time: formatted })
    : t('account.topUp.expiresAt', { time: formatted })
}

function errorMessage(error: unknown) {
  return userFacingErrorCopy(error, t('account.errors.networkRetry'))
}
</script>
