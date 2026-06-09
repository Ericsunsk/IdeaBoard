<template>
  <div class="node-primary-action-slot">
    <NodeActionButton variant="primary" :disabled="disabled" full @click="emit('generate')">
      <span>{{ label }}</span>
      <SendDiagonalSolid class="ml-1.5 size-[15px] translate-x-[0.5px] [stroke-width:1.5]" />
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
