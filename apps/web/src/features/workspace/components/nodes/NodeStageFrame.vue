<template>
  <div
    ref="stageElement"
    class="node-stage-frame"
    :class="[
      `node-stage-frame--${variant}`,
      `node-stage-frame-layout--${layout}`,
      {
        'is-active': active,
        'is-drop-target': dropTarget,
        'is-loading': loading,
        'is-running': running,
        'is-transparent': transparent,
      },
    ]"
    data-flow-handle-frame
    :style="frameStyle"
    :aria-busy="ariaBusy ? 'true' : undefined"
    :aria-label="ariaLabel"
  >
    <slot />
  </div>
</template>

<script setup lang="ts">
import { computed, ref, type StyleValue } from 'vue'

const props = withDefaults(
  defineProps<{
    variant?: 'standard' | 'prompt' | 'square'
    layout?: 'block' | 'center'
    active?: boolean
    dropTarget?: boolean
    loading?: boolean
    running?: boolean
    transparent?: boolean
    height?: number | string
    ariaBusy?: boolean
    ariaLabel?: string
  }>(),
  {
    variant: 'standard',
    layout: 'block',
    active: false,
    dropTarget: false,
    loading: false,
    running: false,
    transparent: false,
    height: undefined,
    ariaBusy: false,
    ariaLabel: undefined,
  },
)

const stageElement = ref<HTMLElement | null>(null)
const frameStyle = computed<StyleValue>(() => {
  if (props.height === undefined) return undefined
  const height = typeof props.height === 'number' ? `${props.height}px` : props.height
  return { height }
})

defineExpose({ stageElement })
</script>

<style scoped>
.node-stage-frame {
  --node-stage-frame-bg: var(--color-canvas-node-bg);
  --node-stage-frame-border: var(--color-canvas-node-border-alt);
  --node-stage-frame-active-border: var(--color-canvas-accent);
  --node-stage-frame-loading-shadow-warm: rgba(239, 68, 68, 0.2);
  --node-stage-frame-loading-shadow-accent: var(--color-canvas-accent-glow);

  position: relative;
  width: 100%;
  border: 1.5px solid var(--node-stage-frame-border);
  border-radius: 11px;
  background: var(--node-stage-frame-bg);
  box-sizing: border-box;
  overflow: hidden;
  transition:
    border-color var(--workspace-motion-standard) ease,
    box-shadow var(--workspace-motion-standard) ease;
}

.node-stage-frame--prompt {
  min-height: 280px;
  border-radius: 16px;
  overflow: visible;
}

.node-stage-frame--square {
  aspect-ratio: 1;
  min-height: 280px;
}

.node-stage-frame-layout--center {
  display: flex;
  align-items: center;
  justify-content: center;
}

.node-stage-frame.is-active,
.node-stage-frame.is-drop-target {
  border-color: var(--node-stage-frame-active-border);
}

.node-stage-frame.is-transparent {
  border-color: transparent;
  background: transparent;
  overflow: visible;
}

.node-stage-frame.is-loading {
  border-color: var(--node-stage-frame-border);
  box-shadow:
    0 0 30px var(--node-stage-frame-loading-shadow-warm),
    0 0 48px var(--node-stage-frame-loading-shadow-accent);
}
</style>
