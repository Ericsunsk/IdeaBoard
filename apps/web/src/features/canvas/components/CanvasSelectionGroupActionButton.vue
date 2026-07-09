<template>
  <div v-if="buttonStyle && action" class="canvas-selection-group-action-layer">
    <button
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
      <span class="node-tool-tooltip" aria-hidden="true">{{ action.label }}</span>
    </button>
  </div>
</template>

<script setup lang="ts">
import { computed, type CSSProperties } from 'vue'
import { ArrowSeparate, Group } from '@iconoir/vue'
import { createPageChromeRoundControlClass } from '../../../shared/lib/pageChrome'
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

const buttonStyle = computed<CSSProperties | null>(() => {
  const position = props.position
  if (!position) return null
  return {
    left: `${position.x}px`,
    top: `${position.y}px`,
  }
})

const actionIcon = computed(() => (props.action?.kind === 'ungroup' ? ArrowSeparate : Group))
</script>

<style scoped>
.canvas-selection-group-action-layer {
  position: absolute;
  inset: 0;
  z-index: 5;
  pointer-events: none;
}

.canvas-selection-group-action-button {
  position: absolute;
  pointer-events: auto;
  transform: translate(-50%, -100%);
}

.canvas-selection-group-action-icon {
  width: var(--canvas-chrome-icon-size);
  height: var(--canvas-chrome-icon-size);
  display: block;
}
</style>
