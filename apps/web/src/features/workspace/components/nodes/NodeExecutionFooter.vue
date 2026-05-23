<template>
  <div class="node-primary-action-slot">
    <NodeActionButton variant="primary" :disabled="disabled" full @click="emit('generate')">
      <span>{{ label }}</span>
      <SendDiagonalSolid class="node-primary-action-icon" />
    </NodeActionButton>
  </div>
</template>

<script setup lang="ts">
import { SendDiagonalSolid } from '@iconoir/vue'

defineProps<{
  label: string
  disabled?: boolean
}>()

const emit = defineEmits<{
  (e: 'generate'): void
}>()
</script>

<style scoped>
.node-primary-action-slot {
  --node-tool-icon-stroke: 1.5;
  padding-top: 0;
}

.node-primary-action-icon {
  width: 15px;
  height: 15px;
  margin-left: 6px;
  stroke-width: var(--node-tool-icon-stroke, 1.5);
  transform: translateX(0.5px);
}
</style>
