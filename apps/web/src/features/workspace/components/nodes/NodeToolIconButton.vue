<template>
  <button
    type="button"
    class="node-tool-icon-button"
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
    <span class="node-tool-icon-button__icon">
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
const resolvedAriaLabel = computed(() => (props.ariaLabelValue ?? resolvedTooltip.value) || undefined)
</script>

<style scoped>
.node-tool-icon-button {
  width: var(--node-tool-icon-button-size, var(--node-tool-button-size, 20px));
  height: var(--node-tool-icon-button-size, var(--node-tool-button-size, 20px));
  border: var(--node-tool-border-width, 1.2px) solid
    var(--node-tool-icon-button-border, var(--node-tool-button-border, rgba(122, 107, 213, 0.28)));
  border-radius: var(--node-tool-icon-button-radius, var(--node-tool-button-radius, 10px));
  background: var(--node-tool-icon-button-bg, #ffffff);
  color: var(--node-tool-icon-button-color, #6f61c6);
  display: inline-grid;
  place-items: center;
  padding: 0;
  box-shadow: none;
  cursor: pointer;
  transition:
    border-color 0.16s ease,
    background-color 0.16s ease,
    color 0.16s ease,
    transform 0.18s ease,
    opacity 0.18s ease;
}

.node-tool-icon-button.active {
  border-color: var(--node-tool-icon-button-active-border, var(--node-tool-button-brand, #7a6bd5));
  color: var(--node-tool-icon-button-active-color, var(--node-tool-button-brand, #7a6bd5));
  background: var(--node-tool-icon-button-active-bg, #ffffff);
}

.node-tool-icon-button:hover:not(:disabled) {
  border-color: var(--node-tool-icon-button-hover-border, var(--node-tool-button-brand, #7a6bd5));
  color: var(--node-tool-icon-button-hover-color, var(--node-tool-button-brand, #7a6bd5));
  background: var(--node-tool-icon-button-hover-bg, #ffffff);
  transform: var(--node-tool-icon-button-hover-transform, scale(1.06));
}

.node-tool-icon-button.primary {
  border-color: var(--node-tool-icon-button-primary-border, var(--node-tool-button-brand, #7a6bd5));
  color: var(--node-tool-icon-button-primary-color, #ffffff);
  background: var(--node-tool-icon-button-primary-bg, var(--node-tool-button-brand, #7a6bd5));
}

.node-tool-icon-button.primary:hover:not(:disabled) {
  border-color: var(--node-tool-icon-button-primary-hover-border, #6c5cb8);
  color: var(--node-tool-icon-button-primary-hover-color, #ffffff);
  background: var(--node-tool-icon-button-primary-hover-bg, #6c5cb8);
}

.node-tool-icon-button:disabled {
  cursor: not-allowed;
  opacity: 0.46;
}

.node-tool-icon-button__icon {
  width: var(--node-tool-icon-button-icon-size, var(--node-tool-icon-size, 21px));
  height: var(--node-tool-icon-button-icon-size, var(--node-tool-icon-size, 21px));
  display: inline-grid;
  place-items: center;
}

.node-tool-icon-button__icon :deep(svg) {
  width: 100%;
  height: 100%;
  display: block;
  stroke-width: var(--node-tool-icon-stroke, 1.5);
}

.node-tool-icon-button__icon :slotted(svg) {
  width: 100%;
  height: 100%;
  display: block;
  stroke-width: var(--node-tool-icon-stroke, 1.5);
}
</style>
<style scoped src="./styles/node-tool-buttons.css"></style>
