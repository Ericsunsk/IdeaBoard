<template>
  <button
    type="button"
    class="node-action-btn hit-target-32-inline inline-flex min-h-7 cursor-pointer items-center justify-center whitespace-nowrap rounded-full border-[length:var(--node-tool-border-width)] border-solid border-[var(--canvas-node-border)] bg-[var(--canvas-node-bg)] px-2.5 text-admin-caption leading-none font-medium text-[var(--canvas-text-option)] transition-[background-color,border-color,color,opacity] duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] disabled:cursor-not-allowed disabled:opacity-[0.45]"
    :class="[
      variantClass,
      {
        active,
        'w-full': full,
        'ml-auto': pushRight,
        'canvas-tool-tooltip-trigger': !!resolvedTooltip,
      },
    ]"
    :disabled="disabled"
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
    active?: boolean
    disabled?: boolean
    full?: boolean
    pushRight?: boolean
    ariaLabel?: string | undefined
    tooltip?: string | undefined
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
  border-radius: var(--canvas-radius-md);
  background: rgba(75, 85, 99, 0.72);
  color: #f9fafb;
  min-height: 26px;
  padding: 0 10px;
  font-weight: 500;
}

.node-action-btn.variant-stage-ghost:hover:not(:disabled) {
  background: rgba(75, 85, 99, 0.88);
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
  height: 24px;
  min-height: 24px;
  padding: 0 9px;
  font-size: 10px;
  font-weight: 500;
  letter-spacing: 0;
}

.node-action-btn.variant-primary {
  min-height: 32px;
  border-color: var(--node-tool-button-brand);
  border-radius: var(--radius-pill);
  background: var(--node-tool-button-brand);
  color: var(--canvas-text-inverse);
  font-size: 12px;
  font-weight: 600;
  letter-spacing: 0;
  padding: 0 12px;
}

.node-action-btn.variant-primary:hover:not(:disabled) {
  background: var(--canvas-accent-strong);
  border-color: var(--canvas-accent-strong);
}
</style>
