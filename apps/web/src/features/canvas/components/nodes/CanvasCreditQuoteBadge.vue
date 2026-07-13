<template>
  <component
    :is="canRetry ? 'button' : 'span'"
    v-if="state"
    :type="canRetry ? 'button' : undefined"
    class="canvas-credit-quote"
    :class="`is-${state.status}`"
    :title="title"
    :aria-label="canRetry ? `${title}，点击重试` : title"
    @click.stop="retry"
  >
    {{ label }}
  </component>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useCanvasBillingReadinessStore, type CanvasBillingQuoteState } from '../../execution'

const props = defineProps<{
  state: CanvasBillingQuoteState | null
}>()

const billingStore = useCanvasBillingReadinessStore()
const canRetry = computed(
  () =>
    !!props.state &&
    (props.state.status === 'blocked' || props.state.status === 'error') &&
    !!billingStore,
)

const label = computed(() => {
  if (!props.state) return ''
  if (props.state.status === 'loading' || props.state.status === 'idle') return '点子…'
  if (props.state.status === 'ready') return `${props.state.totalCredits ?? 0} 点子`
  return props.state.reason || '暂不可用'
})
const title = computed(() => {
  if (!props.state) return ''
  if (props.state.status === 'ready') return `本次预计消耗 ${props.state.totalCredits ?? 0} 点子`
  return props.state.reason || '正在获取点子价格'
})

function retry() {
  if (!props.state || !billingStore) return
  void billingStore.ensureFeature(props.state.feature, { force: true })
}
</script>

<style scoped>
.canvas-credit-quote {
  display: inline-flex;
  min-width: 0;
  max-width: 112px;
  align-items: center;
  justify-content: center;
  border: 1px solid var(--canvas-surface-border-soft);
  border-radius: var(--radius-ib-pill);
  background: var(--canvas-surface-bg-soft);
  padding: 2px 6px;
  color: var(--canvas-text-muted);
  font-size: 10px;
  font-weight: 600;
  line-height: 14px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  cursor: default;
  font-family: inherit;
}

.canvas-credit-quote.is-ready {
  border-color: var(--canvas-accent-outline);
  background: var(--canvas-accent-soft);
  color: var(--canvas-accent-strong);
}

.canvas-credit-quote.is-blocked,
.canvas-credit-quote.is-error {
  border-color: var(--color-ib-danger-border, var(--canvas-surface-border));
  color: var(--color-ib-danger-text, var(--canvas-text-body));
}

button.canvas-credit-quote.is-blocked,
button.canvas-credit-quote.is-error {
  cursor: pointer;
}
</style>
