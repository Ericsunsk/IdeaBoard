<template>
  <button
    type="button"
    class="node-tool-icon-button inline-grid cursor-pointer place-items-center p-0 shadow-none disabled:cursor-not-allowed disabled:opacity-[0.46]"
    :class="{
      active,
      primary,
      'node-tool-tooltip-trigger': !!resolvedTooltip,
    }"
    :style="buttonStyle"
    :disabled="disabled"
    :aria-pressed="active ? 'true' : undefined"
    :aria-label="resolvedAriaLabel"
    @click="emit('click', $event)"
  >
    <span class="node-tool-icon-button__icon inline-grid place-items-center">
      <slot />
    </span>
    <span v-if="resolvedTooltip" class="node-tool-tooltip" aria-hidden="true">
      {{ resolvedTooltip }}
    </span>
  </button>
</template>

<script setup lang="ts">
import { computed } from 'vue'

const props = withDefaults(
  defineProps<{
    size?: number | string
    iconSize?: number | string
    active?: boolean
    disabled?: boolean
    primary?: boolean
    ariaLabelValue?: string
    tooltip?: string
  }>(),
  {
    size: undefined,
    iconSize: undefined,
    active: false,
    disabled: false,
    primary: false,
    ariaLabelValue: undefined,
    tooltip: undefined,
  },
)

const emit = defineEmits<{
  (e: 'click', event: MouseEvent): void
}>()

function toCssSize(value?: number | string) {
  if (typeof value === 'number') return `${value}px`
  return value
}

const buttonStyle = computed(() => ({
  ...(props.size == null ? {} : { '--node-tool-icon-button-size': toCssSize(props.size) }),
  ...(props.iconSize == null
    ? {}
    : { '--node-tool-icon-button-icon-size': toCssSize(props.iconSize) }),
}))
const resolvedTooltip = computed(() => props.tooltip ?? '')
const resolvedAriaLabel = computed(
  () => (props.ariaLabelValue ?? resolvedTooltip.value) || undefined,
)
</script>

<style scoped>
.node-tool-icon-button {
  width: var(--node-tool-icon-button-size);
  height: var(--node-tool-icon-button-size);
  border: var(--node-tool-border-width) solid var(--node-tool-icon-button-border);
  border-radius: var(--node-tool-icon-button-radius);
  background: var(--node-tool-icon-button-bg);
  color: var(--node-tool-icon-button-color);
  transition:
    border-color var(--workspace-motion-fast) ease,
    background-color var(--workspace-motion-fast) ease,
    color var(--workspace-motion-fast) ease,
    transform 0.18s ease,
    opacity 0.18s ease;
}

.node-tool-icon-button.active {
  border-color: var(--node-tool-icon-button-active-border);
  color: var(--node-tool-icon-button-active-color);
  background: var(--node-tool-icon-button-active-bg);
}

.node-tool-icon-button:hover:not(:disabled) {
  border-color: var(--node-tool-icon-button-hover-border);
  color: var(--node-tool-icon-button-hover-color);
  background: var(--node-tool-icon-button-hover-bg);
  transform: var(--node-tool-icon-button-hover-transform);
}

.node-tool-icon-button.primary {
  border-color: var(--node-tool-icon-button-primary-border);
  color: var(--node-tool-icon-button-primary-color);
  background: var(--node-tool-icon-button-primary-bg);
}

.node-tool-icon-button.primary:hover:not(:disabled) {
  border-color: var(--node-tool-icon-button-primary-hover-border);
  color: var(--node-tool-icon-button-primary-hover-color);
  background: var(--node-tool-icon-button-primary-hover-bg);
}

.node-tool-icon-button__icon {
  width: var(--node-tool-icon-button-icon-size);
  height: var(--node-tool-icon-button-icon-size);
}

.node-tool-icon-button__icon :deep(svg) {
  width: 100%;
  height: 100%;
  display: block;
}

.node-tool-icon-button__icon :slotted(svg) {
  width: 100%;
  height: 100%;
  display: block;
}
</style>
<style scoped src="./styles/node-tool-buttons.css"></style>
