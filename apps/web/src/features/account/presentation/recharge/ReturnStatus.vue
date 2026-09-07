<template>
  <section
    ref="statusElement"
    tabindex="-1"
    class="rounded-[var(--radius-ib-panel)] border px-4 py-3 focus-visible:outline-none focus-visible:shadow-[var(--shadow-ib-focus)]"
    :class="statusClass"
    :role="showError ? 'alert' : 'status'"
    aria-live="polite"
  >
    <template v-if="status?.status === 'credited'">
      <p class="m-0 font-semibold text-ib-success">
        {{
          t('account.returnStatus.creditedTitle', {
            amount: formatIdeaAmount(status.idea_quantity),
          })
        }}
      </p>
      <p class="mt-1 mb-0 text-sm text-ib-text-muted">
        {{ t('account.returnStatus.creditedDetail') }}
      </p>
    </template>
    <template v-else-if="source === 'cancel'">
      <p class="m-0 font-semibold text-ib-text">{{ t('account.returnStatus.returnedTitle') }}</p>
      <p class="mt-1 mb-0 text-sm text-ib-text-muted">
        {{ t('account.returnStatus.returnedDetail') }}
      </p>
    </template>
    <template v-else-if="pending">
      <p class="m-0 font-semibold text-ib-text">{{ t('account.returnStatus.confirmingTitle') }}</p>
      <p class="mt-1 mb-0 text-sm text-ib-text-muted">
        {{ t('account.returnStatus.confirmingDetail') }}
      </p>
    </template>
    <template v-else-if="error">
      <p class="m-0 font-semibold text-ib-danger">
        {{ t('account.errors.orderStatusLoadFailed') }}
      </p>
      <Button
        :label="t('account.returnStatus.recheck')"
        severity="secondary"
        size="small"
        text
        class="mt-2"
        @click="emit('retry')"
      />
    </template>
    <template v-else-if="status?.status === 'failed'">
      <p class="m-0 font-semibold text-ib-danger">{{ t('account.returnStatus.failedTitle') }}</p>
      <p class="mt-1 mb-0 text-sm text-ib-text-muted">
        {{ t('account.returnStatus.failedDetail') }}
      </p>
    </template>
    <template v-else-if="delayed">
      <p class="m-0 font-semibold text-ib-text">{{ t('account.returnStatus.delayedTitle') }}</p>
      <p class="mt-1 mb-0 text-sm text-ib-text-muted">
        {{ t('account.returnStatus.delayedDetail') }}
      </p>
    </template>
    <template v-else>
      <p class="m-0 font-semibold text-ib-text">{{ t('account.returnStatus.confirmingTitle') }}</p>
      <p class="mt-1 mb-0 text-sm text-ib-text-muted">
        {{ t('account.returnStatus.confirmingDetail') }}
      </p>
    </template>
  </section>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Button from 'primevue/button'
import { formatIdeaAmount } from '../../../../shared/i18n/userIdeaCurrency'
import type { PaymentReturnSource } from '../../domain'
import type { PaymentOrderReturnStatus } from '../../infrastructure'

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
let delayedTimer: ReturnType<typeof setTimeout> | undefined
const showError = computed(
  () =>
    props.status?.status !== 'credited' &&
    props.source !== 'cancel' &&
    props.error !== null &&
    props.error !== undefined,
)
const statusClass = computed(() => {
  if (props.status?.status === 'credited') {
    return 'border-ib-success/25 bg-ib-surface text-ib-text'
  }
  if (props.source === 'cancel') {
    return 'border-ib-border-soft bg-ib-surface text-ib-text'
  }
  if (props.error || props.status?.status === 'failed') {
    return 'border-ib-danger/25 bg-ib-surface text-ib-text'
  }
  return 'border-ib-border-soft bg-ib-surface text-ib-text'
})

onMounted(async () => {
  delayedTimer = setTimeout(() => {
    delayed.value = true
  }, 60_000)
  await nextTick()
  statusElement.value?.focus()
})

onBeforeUnmount(() => {
  if (delayedTimer) clearTimeout(delayedTimer)
})
</script>
