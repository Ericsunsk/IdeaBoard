<template>
  <button
    type="button"
    class="node-action-btn hit-target-32-inline inline-flex min-h-7 cursor-pointer items-center justify-center whitespace-nowrap rounded-pill border-[length:var(--node-tool-border-width)] border-solid border-[var(--canvas-node-border)] bg-[var(--canvas-node-bg)] px-2.5 text-admin-caption leading-none font-medium text-[var(--canvas-text-option)] transition-[background-color,border-color,color,opacity] duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] disabled:cursor-not-allowed disabled:opacity-[0.45]"
    :class="[
      variantClass,
      {
        isActive,
        'w-full': isFull,
        'ml-auto': isPushRight,
        'canvas-tool-tooltip-trigger': !!resolvedTooltip,
      },
    ]"
    :disabled="isDisabled"
    :aria-label="resolvedAriaLabel"
    @click="emit('click', $event)"
  >
    <slot />
    <span v-if="resolvedTooltip" class="canvas-tool-tooltip" aria-hidden="true">
      {{ resolvedTooltip }}
    </span>
  </button>
</template>

<script setup lang="ts">
import { computed } from 'vue'

const props = withDefaults(
  defineProps<{
    variant?: NodeActionButtonVariant
    isActive?: boolean
    isDisabled?: boolean
    isFull?: boolean
    isPushRight?: boolean
    ariaLabel?: string | undefined
    tooltip?: string | undefined
  }>(),
  {
    variant: 'tool',
    isActive: false,
    isDisabled: false,
    isFull: false,
    isPushRight: false,
    ariaLabel: undefined,
    tooltip: undefined,
  },
)

const emit = defineEmits<{
  (e: 'click', event: MouseEvent): void
}>()

type NodeActionButtonVariant = 'stage-ghost' | 'tool' | 'chip' | 'primary'

const nodeActionButtonVariantClasses: Record<NodeActionButtonVariant, string> = {
  'stage-ghost': 'variant-stage-ghost',
  tool: 'variant-tool',
  chip: 'variant-chip',
  primary: 'variant-primary',
}

const resolvedTooltip = computed(() => props.tooltip ?? '')
const resolvedAriaLabel = computed(() => (props.ariaLabel ?? resolvedTooltip.value) || undefined)
const variantClass = computed(() => nodeActionButtonVariantClasses[props.variant])
</script>

<style scoped>
.node-action-btn {
  --node-tool-border-width: var(--canvas-control-border-width);
  --node-tool-button-accent: var(--canvas-accent);
  --node-tool-button-brand: var(--canvas-accent-hover);
}

.node-action-btn.variant-stage-ghost {
  border: none;
  border-radius: var(--radius-control);
  background: color-mix(in srgb, var(--color-media-scrim) 72%, transparent);
  color: var(--color-media-foreground);
  min-height: 26px;
  padding: 0 10px;
  font-weight: 500;
}

.node-action-btn.variant-stage-ghost:hover:not(:disabled) {
  background: color-mix(in srgb, var(--color-media-scrim) 88%, transparent);
}

.node-action-btn.variant-tool:hover:not(:disabled),
.node-action-btn.variant-chip:hover:not(:disabled) {
  border-color: var(--node-tool-button-accent);
  color: var(--canvas-node-text-primary);
}

.node-action-btn.variant-tool.active,
.node-action-btn.variant-chip.active {
  border-color: var(--node-tool-button-accent);
  background: var(--canvas-node-accent-soft);
  color: var(--canvas-node-text-primary);
}

.node-action-btn.variant-chip {
  height: var(--spacing-touch-target-aa);
  min-height: var(--spacing-touch-target-aa);
  padding: 0 9px;
  font-size: var(--canvas-font-size-micro);
  font-weight: 500;
  letter-spacing: 0;
}

.node-action-btn.variant-primary {
  min-height: var(--spacing-touch-target-min);
  border-color: var(--node-tool-button-brand);
  border-radius: var(--radius-pill);
  background: var(--node-tool-button-brand);
  color: var(--canvas-text-inverse);
  font-size: var(--canvas-font-size-control);
  font-weight: 600;
  letter-spacing: 0;
  padding: 0 12px;
}

.node-action-btn.variant-primary:hover:not(:disabled) {
  background: var(--canvas-accent-strong);
  border-color: var(--canvas-accent-strong);
}
</style>
