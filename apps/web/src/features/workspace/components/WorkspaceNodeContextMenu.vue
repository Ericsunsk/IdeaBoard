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
    class="workspace-node-context-menu absolute z-[160] w-40 min-w-40 max-w-40 rounded-[10px] bg-white/[0.96] p-1.5 backdrop-blur-[10px] [border:var(--canvas-control-border-width,1.2px)_solid_#d8ddea]"
    :style="{ left: `${x}px`, top: `${y}px` }"
    @mousedown.stop
    @click.stop
  >
    <button
      v-for="item in items"
      :key="item.key"
      type="button"
      class="h-[34px] w-full cursor-pointer rounded-lg border-0 bg-transparent px-2.5 text-left text-[13px] font-semibold text-[#4b5565] hover:bg-[rgba(146,120,230,0.12)] hover:text-[#6757c8]"
      @pointerdown.stop.prevent="emit('create', item.key)"
    >
      {{ item.label }}
    </button>
  </div>
</template>

<script setup lang="ts">
import type { FlowCreatableNodeKind } from '../composables/useFlowNodeMutations'

const props = defineProps<{
  visible: boolean
  x: number
  y: number
  items: ReadonlyArray<{ key: FlowCreatableNodeKind; label: string }>
}>()

const emit = defineEmits<{
  create: [kind: FlowCreatableNodeKind]
}>()

void props
</script>
