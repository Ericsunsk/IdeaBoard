<template>
  <div
    ref="stageElement"
    class="node-stage-frame"
    :class="[
      `node-stage-frame--${variant}`,
      `node-stage-frame-layout--${layout}`,
      {
        'is-active': isActive,
        'is-drop-target': isDropTarget,
        'is-connection-drop-target': isConnectionDropTarget,
        'is-loading': isLoading,
        'is-running': isRunning,
        'is-transparent': isTransparent,
      },
    ]"
    :style="frameStyle"
    :aria-busy="isAriaBusy ? 'true' : undefined"
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
    isActive?: boolean
    isDropTarget?: boolean
    isConnectionDropTarget?: boolean
    isLoading?: boolean
    isRunning?: boolean
    isTransparent?: boolean
    height?: number | string | undefined
    isAriaBusy?: boolean
    ariaLabel?: string | undefined
  }>(),
  {
    variant: 'standard',
    layout: 'block',
    isActive: false,
    isDropTarget: false,
    isConnectionDropTarget: false,
    isLoading: false,
    isRunning: false,
    isTransparent: false,
    height: undefined,
    isAriaBusy: false,
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
  --node-stage-frame-bg: var(--canvas-node-bg);
  --node-stage-frame-active-border: var(--canvas-accent);
  --node-stage-frame-emphasis-shadow-primary: var(--canvas-accent-shadow);
  --node-stage-frame-emphasis-shadow-secondary: var(--canvas-accent-glow);

  position: relative;
  width: 100%;
  border: 0;
  border-radius: var(--radius-panel);
  background: var(--node-stage-frame-bg);
  box-sizing: border-box;
  overflow: hidden;
  transition: box-shadow var(--motion-feedback-duration) var(--motion-feedback-easing);
}

.node-stage-frame::after {
  position: absolute;
  z-index: var(--canvas-z-stage);
  inset: 0;
  border-radius: inherit;
  box-shadow: inset 0 0 0 1.5px var(--node-stage-frame-active-border);
  content: '';
  opacity: 0;
  pointer-events: none;
  transition: opacity var(--motion-feedback-duration) var(--motion-feedback-easing);
}

.node-stage-frame--prompt {
  min-height: 280px;
  border-radius: var(--radius-overlay);
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
  &::after {
    opacity: 1;
  }
}

.node-stage-frame.is-transparent {
  background: transparent;
  overflow: visible;

  &::after {
    opacity: 0;
  }
}

.node-stage-frame.is-loading,
.node-stage-frame.is-connection-drop-target {
  --node-stage-frame-emphasis-shadow-primary: color-mix(
    in srgb,
    var(--canvas-accent) 45%,
    transparent
  );
  --node-stage-frame-emphasis-shadow-secondary: color-mix(
    in srgb,
    var(--canvas-accent) 25%,
    transparent
  );
  box-shadow:
    0 0 30px var(--node-stage-frame-emphasis-shadow-primary),
    0 0 48px var(--node-stage-frame-emphasis-shadow-secondary);
}
</style>
