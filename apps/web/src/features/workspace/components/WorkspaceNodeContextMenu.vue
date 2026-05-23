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
    class="node-context-menu"
    :style="{ left: `${x}px`, top: `${y}px` }"
    @mousedown.stop
    @click.stop
  >
    <button
      v-for="item in items"
      :key="item.key"
      type="button"
      class="node-context-item"
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

<style scoped>
.node-context-menu {
  position: absolute;
  z-index: 160;
  width: 160px;
  min-width: 160px;
  max-width: 160px;
  padding: 6px;
  border-radius: 10px;
  border: var(--canvas-control-border-width, 1.2px) solid #d8ddea;
  background: rgba(255, 255, 255, 0.96);
  backdrop-filter: blur(10px);
}

.node-context-item {
  width: 100%;
  height: 34px;
  border: none;
  border-radius: 8px;
  background: transparent;
  text-align: left;
  padding: 0 10px;
  font-size: 13px;
  font-weight: 600;
  color: #4b5565;
  cursor: pointer;
}

.node-context-item:hover {
  background: rgba(146, 120, 230, 0.12);
  color: #6757c8;
}
</style>
