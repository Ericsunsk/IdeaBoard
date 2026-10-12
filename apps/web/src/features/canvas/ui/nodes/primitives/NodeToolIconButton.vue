<template>
  <button
    type="button"
    v-bind="controlStateAttrs"
    class="control-state node-tool-icon-button inline-grid cursor-pointer place-items-center p-0 shadow-none"
    :class="{
      isPrimary,
      'canvas-tool-tooltip-trigger': !!resolvedTooltip,
    }"
    :style="buttonStyle"
    :aria-label="resolvedAriaLabel"
    @click="emit('click', $event)"
  >
    <span class="node-tool-icon-button__icon inline-grid place-items-center">
      <slot />
    </span>
    <span v-if="resolvedTooltip" class="canvas-tool-tooltip" aria-hidden="true">
      {{ resolvedTooltip }}
    </span>
  </button>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { resolveInteractiveControlState } from '../../../../../core/interaction/interactiveControlState'

const props = withDefaults(
  defineProps<{
    size?: number | string | undefined
    iconSize?: number | string | undefined
    isActive?: boolean
    isDisabled?: boolean
    isPrimary?: boolean
    ariaLabel?: string | undefined
    tooltip?: string | undefined
  }>(),
  {
    size: undefined,
    iconSize: undefined,
    isActive: false,
    isDisabled: false,
    isPrimary: false,
    ariaLabel: undefined,
    tooltip: undefined,
  },
)

const emit = defineEmits<{
  (e: 'click', event: MouseEvent): void
}>()

defineOptions({
  inheritAttrs: false,
})

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
const resolvedAriaLabel = computed(() => (props.ariaLabel ?? resolvedTooltip.value) || undefined)
const controlStateAttrs = computed(() =>
  resolveInteractiveControlState({
    isDisabled: props.isDisabled,
    isSelected: props.isActive,
  }),
)
</script>

<style scoped>
.node-tool-icon-button {
  --control-disabled-opacity: 0.46;
  --control-hover-transform: var(--node-tool-icon-button-hover-transform);
  width: var(--node-tool-icon-button-size);
  height: var(--node-tool-icon-button-size);
  border: var(--node-tool-border-width) solid var(--node-tool-icon-button-border);
  border-radius: var(--node-tool-icon-button-radius);
  background: var(--node-tool-icon-button-bg);
  backdrop-filter: blur(var(--canvas-glass-blur));
  -webkit-backdrop-filter: blur(var(--canvas-glass-blur));
  color: var(--node-tool-icon-button-color);
  box-shadow: none;
  transition:
    border-color var(--motion-control-duration) var(--motion-control-easing),
    background-color var(--motion-control-duration) var(--motion-control-easing),
    color var(--motion-control-duration) var(--motion-control-easing),
    transform var(--motion-control-duration) var(--motion-control-easing),
    opacity var(--motion-feedback-duration) var(--motion-feedback-easing);
}

.node-tool-icon-button[aria-pressed='true'] {
  border-color: var(--node-tool-icon-button-active-border);
  color: var(--node-tool-icon-button-active-color);
  background: var(--node-tool-icon-button-active-bg);
}

.node-tool-icon-button:hover:not(:disabled) {
  border-color: var(--node-tool-icon-button-hover-border);
  color: var(--node-tool-icon-button-hover-color);
  background: var(--node-tool-icon-button-hover-bg);
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

/* Slot icon contract: external icon components render SVGs inside this slot. */
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
