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
        <component :is="actionIcon" class="canvas-selection-group-action-icon" aria-hidden="true" />
        <span class="canvas-tool-tooltip" aria-hidden="true">{{ action.label }}</span>
      </button>
    </Transition>
  </div>
</template>

<script setup lang="ts">
import { computed, type CSSProperties } from 'vue'
import { KeyframesMinus, KeyframesPlus } from '@iconoir/vue'
import { pageChromeClasses } from '../../../../core/lib/pageChrome'
import { cn } from '../../../../lib/utils'
import { createPopoverMotionTransition } from '../../../../core/motion/popoverMotion'
import type { CanvasSelectionGroupAction } from '../../session/graph-editing/groupContracts'

const props = defineProps<{
  action: CanvasSelectionGroupAction | null
  position: { x: number; y: number } | null
}>()

const emit = defineEmits<{
  (event: 'activate'): void
}>()

const actionButtonClass = cn(
  pageChromeClasses.roundControl,
  'canvas-selection-group-action-button',
  'canvas-tool-tooltip-trigger',
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
  z-index: var(--canvas-z-action);
  pointer-events: none;
}

.canvas-selection-group-action-button {
  --popover-motion-from-transform: scale(0.96);
  position: absolute;
  pointer-events: auto;
  translate: -50% -100%;
}

.canvas-selection-group-action-icon {
  width: var(--page-chrome-icon-size, 19px);
  height: var(--page-chrome-icon-size, 19px);
  display: block;
}
</style>
