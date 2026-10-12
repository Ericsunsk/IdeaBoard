<template>
  <div
    class="workflow-node flex flex-col gap-0.5 overflow-visible border-0 bg-transparent shadow-none"
    :class="[
      `${kind}-node`,
      {
        isActive,
        isDragging,
        'copy-mode': isCopyMode,
      },
    ]"
    :data-node-kind="kind"
    :data-node-id="nodeId"
    :style="nodeStyle"
  >
    <NodeHeader
      :class="{ 'invisible pointer-events-none': isHeaderHidden }"
      :title="title"
      :is-active="isActive"
      :is-copy-mode="isCopyMode"
      :is-rounded="isRoundedHeader"
      @node-mousedown="emit('node-mousedown', $event)"
      @update:title="emit('update:title', $event)"
      @delete="emit('delete')"
    >
      <template v-if="$slots.icon" #icon>
        <slot name="icon" />
      </template>
    </NodeHeader>
    <slot />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import NodeHeader from './NodeHeader.vue'

const props = defineProps<{
  kind: 'prompt' | 'image' | 'fusion'
  nodeId: string | number
  width: number
  title: string
  isActive: boolean
  isDragging: boolean
  isCopyMode: boolean
  isRoundedHeader?: boolean
  isHeaderHidden?: boolean
}>()

const emit = defineEmits<{
  (e: 'node-mousedown', event: MouseEvent): void
  (e: 'update:title', value: string): void
  (e: 'delete'): void
}>()

const nodeStyle = computed(() => ({
  width: `${props.width}px`,
}))
</script>

<style scoped>
.workflow-node {
  position: relative;
  z-index: 1;
  pointer-events: auto;
}

.workflow-node.dragging {
  transition: none;
}
</style>
