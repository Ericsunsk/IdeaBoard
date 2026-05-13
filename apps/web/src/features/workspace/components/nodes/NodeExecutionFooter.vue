<template>
  <NodeConnectionChips :items="items" />
  <div class="node-primary-action-slot">
    <NodeActionButton variant="primary" :disabled="disabled" full @click="emit('generate')">
      {{ label }}
    </NodeActionButton>
  </div>
</template>

<script setup lang="ts">
type ConnectionChipItem = {
  key: string
  label: string
  kind: 'image' | 'mask' | 'prompt'
  connected?: boolean
  pending?: boolean
  error?: boolean
  required?: boolean
  visible?: boolean
}

defineProps<{
  items: ConnectionChipItem[]
  label: string
  disabled?: boolean
}>()

const emit = defineEmits<{
  (e: 'generate'): void
}>()
</script>

<style scoped>
.node-primary-action-slot {
  padding-top: 2px;
}
</style>
