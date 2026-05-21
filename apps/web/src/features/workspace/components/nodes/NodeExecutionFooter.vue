<template>
  <div class="node-primary-action-slot">
    <NodeActionButton variant="primary" :disabled="disabled" full @click="emit('generate')">
      {{ label }}
    </NodeActionButton>
  </div>
</template>

<script setup lang="ts">
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
  padding-top: 2px;
}
</style>
