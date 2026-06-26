<template>
  <div
    v-if="visible"
    v-motion="{
      initial: { opacity: 0, y: 6, scale: 0.985 },
      enter: {
        opacity: 1,
        y: 0,
        scale: 1,
        transition: { duration: 150, easing: 'ease-out' },
      },
    }"
    class="workspace-node-context-menu absolute z-[160] w-40 min-w-40 max-w-40 rounded-[var(--workspace-surface-radius)] bg-[var(--workspace-surface-bg)] p-1.5 backdrop-blur-[var(--workspace-surface-blur)] [border:var(--workspace-control-border-width)_solid_var(--workspace-surface-border)]"
    :style="{ left: `${x}px`, top: `${y}px` }"
    @mousedown.stop
    @click.stop
  >
    <button
      v-for="item in items"
      :key="item.key"
      type="button"
      class="h-[34px] w-full cursor-pointer rounded-lg border-0 bg-transparent px-2.5 text-left text-[13px] font-semibold text-[var(--workspace-text-body)] hover:bg-[var(--workspace-accent-soft)] hover:text-[var(--workspace-accent-strong)]"
      @pointerdown.stop.prevent="emit('create', item.key)"
    >
      {{ item.label }}
    </button>
  </div>
</template>

<script setup lang="ts">
import type { CanvasCreatableNodeKind } from '../config/canvasNodeRegistry'

const props = defineProps<{
  visible: boolean
  x: number
  y: number
  items: ReadonlyArray<{ key: CanvasCreatableNodeKind; label: string }>
}>()

const emit = defineEmits<{
  create: [kind: CanvasCreatableNodeKind]
}>()

void props
</script>
