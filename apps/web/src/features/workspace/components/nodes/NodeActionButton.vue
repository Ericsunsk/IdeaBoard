<template>
  <button
    type="button"
    class="node-action-btn"
    :class="[
      `variant-${variant}`,
      { active, full, 'push-right': pushRight, 'node-tool-tooltip-trigger': !!resolvedTooltip },
    ]"
    :disabled="disabled"
    :aria-label="resolvedAriaLabel"
    @click="emit('click', $event)"
  >
    <slot />
    <span v-if="resolvedTooltip" class="node-tool-tooltip" aria-hidden="true">
      {{ resolvedTooltip }}
    </span>
  </button>
</template>

<script setup lang="ts">
import { computed } from 'vue'

const props = withDefaults(
  defineProps<{
    variant?: 'stage' | 'stage-ghost' | 'tool' | 'chip' | 'primary'
    active?: boolean
    disabled?: boolean
    full?: boolean
    pushRight?: boolean
    ariaLabel?: string
    tooltip?: string
  }>(),
  {
    variant: 'tool',
    active: false,
    disabled: false,
    full: false,
    pushRight: false,
    ariaLabel: undefined,
    tooltip: undefined,
  },
)

const emit = defineEmits<{
  (e: 'click', event: MouseEvent): void
}>()

const resolvedTooltip = computed(() => props.tooltip ?? '')
const resolvedAriaLabel = computed(() => (props.ariaLabel ?? resolvedTooltip.value) || undefined)
</script>

<style scoped src="./styles/node-tool-buttons.css"></style>
<style scoped>
.node-action-btn {
  border: var(--node-tool-border-width, 1.2px) solid #d6dcec;
  background: #ffffff;
  color: #5f6877;
  border-radius: 999px;
  min-height: 28px;
  padding: 0 10px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  font-size: 11px;
  line-height: 1;
  font-weight: 500;
  white-space: nowrap;
  cursor: pointer;
  transition: all 0.2s ease;
}

.node-action-btn.full {
  width: 100%;
}

.node-action-btn.push-right {
  margin-left: auto;
}

.node-action-btn.variant-stage {
  border: none;
  border-radius: 8px;
  background: rgba(17, 24, 39, 0.74);
  color: #f9fafb;
  min-height: 26px;
  padding: 0 10px;
  font-weight: 500;
}

.node-action-btn.variant-stage:hover:not(:disabled) {
  background: rgba(17, 24, 39, 0.9);
}

.node-action-btn.variant-stage-ghost {
  border: none;
  border-radius: 8px;
  background: rgba(75, 85, 99, 0.72);
  color: #f9fafb;
  min-height: 26px;
  padding: 0 10px;
  font-weight: 500;
}

.node-action-btn.variant-stage-ghost:hover:not(:disabled) {
  background: rgba(75, 85, 99, 0.88);
}

.node-action-btn.variant-tool:hover:not(:disabled) {
  border-color: var(--node-tool-button-accent, #9278e6);
  color: #272932;
}

.node-action-btn.variant-tool.active {
  border-color: var(--node-tool-button-accent, #9278e6);
  background: #f8f7ff;
  color: #272932;
}

.node-action-btn.variant-chip {
  height: 24px;
  min-height: 24px;
  padding: 0 9px;
  font-size: 10px;
  font-weight: 500;
  letter-spacing: 0;
}

.node-action-btn.variant-chip:hover:not(:disabled) {
  border-color: var(--node-tool-button-accent, #9278e6);
  color: #272932;
}

.node-action-btn.variant-chip.active {
  border-color: var(--node-tool-button-accent, #9278e6);
  background: #f8f7ff;
  color: #272932;
}

.node-action-btn.variant-primary {
  min-height: 32px;
  border-color: var(--node-tool-button-brand, #7a6bd5);
  border-radius: 999px;
  background: var(--node-tool-button-brand, #7a6bd5);
  color: #ffffff;
  font-size: 12px;
  font-weight: 600;
  letter-spacing: 0;
  padding: 0 12px;
}

.node-action-btn.variant-primary:hover:not(:disabled) {
  background: #7f68db;
  border-color: #7f68db;
}

.node-action-btn:disabled {
  cursor: not-allowed;
  opacity: 0.45;
}
</style>
