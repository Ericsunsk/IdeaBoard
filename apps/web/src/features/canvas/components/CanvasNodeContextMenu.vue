<template>
  <Transition v-bind="transientOverlayMotion">
    <div
      v-if="visible"
      class="canvas-node-context-menu absolute z-[160] w-40 min-w-40 max-w-40 rounded-[var(--canvas-surface-radius)] bg-[var(--canvas-surface-bg)] p-1.5 backdrop-blur-[var(--canvas-surface-blur)] [border:var(--canvas-control-border-width)_solid_var(--canvas-surface-border)]"
      :style="{ left: `${x}px`, top: `${y}px` }"
      @mousedown.stop
      @click.stop
    >
      <button
        v-for="item in items"
        :key="item.key"
        type="button"
        class="h-[34px] w-full cursor-pointer rounded-lg border-0 bg-transparent px-2.5 text-left text-[13px] font-semibold text-[var(--canvas-text-body)] hover:bg-[var(--canvas-accent-soft)] hover:text-[var(--canvas-accent-strong)]"
        @pointerdown.stop.prevent="emit('create', item.key)"
      >
        {{ item.label }}
      </button>
    </div>
  </Transition>
</template>

<script setup lang="ts">
import type { CanvasCreatableNodeKind } from '../../canvas-board/domain'
import { createPopoverMotionTransition } from '../../../shared/motion/popoverMotion'

const props = defineProps<{
  visible: boolean
  x: number
  y: number
  items: ReadonlyArray<{ key: CanvasCreatableNodeKind; label: string }>
}>()

const emit = defineEmits<{
  create: [kind: CanvasCreatableNodeKind]
}>()

const transientOverlayMotion = createPopoverMotionTransition()

void props
</script>
