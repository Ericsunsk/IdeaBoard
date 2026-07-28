<template>
  <div class="canvas-selection-group-action-layer">
    <Transition v-bind="groupActionMotion">
      <button
        v-if="buttonStyle && action"
        type="button"
        :class="actionButtonClass"
        :style="buttonStyle"
        :aria-label="action.label"
        :title="action.label"
        @pointerdown.stop
        @mousedown.stop
        @contextmenu.stop.prevent
        @click.stop="emit('activate')"
      >
        <component
          :is="actionIcon"
          class="canvas-selection-group-action-icon"
          aria-hidden="true"
        />
        <span class="node-tool-tooltip" aria-hidden="true">{{ action.label }}</span>
      </button>
    </Transition>
  </div>
</template>

<script setup lang="ts">
import { computed, type CSSProperties } from 'vue'
import { KeyframesMinus, KeyframesPlus } from '@iconoir/vue'
import { createPageChromeRoundControlClass } from '../../../shared/lib/pageChrome'
import { createPopoverMotionTransition } from '../../../shared/motion/popoverMotion'
import type { CanvasSelectionGroupAction } from '../composables/useFlowCanvasRuntime'

const props = defineProps<{
  action: CanvasSelectionGroupAction | null
  position: { x: number; y: number } | null
}>()

const emit = defineEmits<{
  (event: 'activate'): void
}>()

const actionButtonClass = createPageChromeRoundControlClass(
  'canvas-chrome-icon-button',
  'canvas-selection-group-action-button',
  'node-tool-tooltip-trigger',
)
const groupActionMotion = createPopoverMotionTransition()

const buttonStyle = computed<CSSProperties | null>(() => {
  const position = props.position
  if (!position) return null
  return {
    left: `${position.x}px`,
    top: `${position.y}px`,
  }
})

const actionIcon = computed(() =>
  props.action?.kind === 'ungroup' ? KeyframesMinus : KeyframesPlus,
)
</script>

<style scoped>
.canvas-selection-group-action-layer {
  position: absolute;
  inset: 0;
  z-index: 5;
  pointer-events: none;
}

.canvas-selection-group-action-button {
  --ib-popover-motion-from-transform: scale(0.96);
  position: absolute;
  pointer-events: auto;
  translate: -50% -100%;
}

.canvas-selection-group-action-icon {
  width: var(--canvas-chrome-icon-size);
  height: var(--canvas-chrome-icon-size);
  display: block;
}
</style>
