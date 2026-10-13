<template>
  <section
    ref="statusElement"
    tabindex="-1"
    class="rounded-panel border px-4 py-3 focus-visible:outline-none focus-visible:shadow-focus-ring"
    :class="statusClass"
    :role="showError ? 'alert' : 'status'"
    aria-live="polite"
  >
    <template v-if="status?.status === 'credited'">
      <p class="m-0 font-semibold text-success">
        {{
          t('account.returnStatus.creditedTitle', {
            amount: formatIdeaAmount(status.idea_quantity),
          })
        }}
      </p>
      <p class="mt-1 mb-0 text-sm text-muted-foreground">
        {{ t('account.returnStatus.creditedDetail') }}
      </p>
    </template>
    <template v-else-if="source === 'cancel'">
      <p class="m-0 font-semibold text-foreground">{{ t('account.returnStatus.returnedTitle') }}</p>
      <p class="mt-1 mb-0 text-sm text-muted-foreground">
        {{ t('account.returnStatus.returnedDetail') }}
      </p>
    </template>
    <template v-else-if="pending">
      <p class="m-0 font-semibold text-foreground">
        {{ t('account.returnStatus.confirmingTitle') }}
      </p>
      <p class="mt-1 mb-0 text-sm text-muted-foreground">
        {{ t('account.returnStatus.confirmingDetail') }}
      </p>
    </template>
    <template v-else-if="error">
      <p class="m-0 font-semibold text-destructive">
        {{ t('account.errors.orderStatusLoadFailed') }}
      </p>
      <Button variant="ghost" size="sm" class="mt-2" @click="emit('retry')">{{
        t('account.returnStatus.recheck')
      }}</Button>
    </template>
    <template v-else-if="status?.status === 'failed'">
      <p class="m-0 font-semibold text-destructive">{{ t('account.returnStatus.failedTitle') }}</p>
      <p class="mt-1 mb-0 text-sm text-muted-foreground">
        {{ t('account.returnStatus.failedDetail') }}
      </p>
    </template>
    <template v-else-if="delayed">
      <p class="m-0 font-semibold text-foreground">{{ t('account.returnStatus.delayedTitle') }}</p>
      <p class="mt-1 mb-0 text-sm text-muted-foreground">
        {{ t('account.returnStatus.delayedDetail') }}
      </p>
    </template>
    <template v-else>
      <p class="m-0 font-semibold text-foreground">
        {{ t('account.returnStatus.confirmingTitle') }}
      </p>
      <p class="mt-1 mb-0 text-sm text-muted-foreground">
        {{ t('account.returnStatus.confirmingDetail') }}
      </p>
    </template>
  </section>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, ref } from 'vue'
import { useTimeoutFn } from '@vueuse/core'
import { useI18n } from 'vue-i18n'
import { Button } from '@/components/ui/button'
import { formatIdeaAmount } from '../../../../core/i18n/userIdeaCurrency'
import type { PaymentReturnSource } from '../../core/return'
import type { PaymentOrderReturnStatus } from '../../session/payments/resource'

const props = defineProps<{
  pending: boolean
  error: unknown
  source: PaymentReturnSource
  status?: PaymentOrderReturnStatus | undefined
}>()
const emit = defineEmits<{ retry: [] }>()
const { t } = useI18n()
const statusElement = ref<HTMLElement>()
const delayed = ref(false)
// `useTimeoutFn` auto-cancels on unmount — no manual clearTimeout pairing.
useTimeoutFn(() => {
  delayed.value = true
}, 60_000)
const showError = computed(
  () =>
    props.status?.status !== 'credited' &&
    props.source !== 'cancel' &&
    props.error !== null &&
    props.error !== undefined,
)
const statusClass = computed(() => {
  if (props.status?.status === 'credited') {
    return 'border-emerald-500/25 bg-card text-foreground'
  }
  if (props.source === 'cancel') {
    return 'border-border bg-card text-foreground'
  }
  if (props.error || props.status?.status === 'failed') {
    return 'border-destructive/25 bg-card text-foreground'
  }
  return 'border-border bg-card text-foreground'
})

onMounted(async () => {
  await nextTick()
  statusElement.value?.focus()
})
</script>
