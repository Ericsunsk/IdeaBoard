<template>
  <div
    class="workflow-node-header"
    :class="{ 'workflow-node-header--rounded': rounded }"
    @mousedown="emit('node-mousedown', $event)"
  >
    <div class="node-header-title">
      <span v-if="$slots.icon" class="node-header-icon">
        <slot name="icon" />
      </span>
      {{ title }}
    </div>
  </div>
</template>

<script setup lang="ts">
defineProps<{
  title: string
  rounded?: boolean
}>()

const emit = defineEmits<{
  (e: 'node-mousedown', event: MouseEvent): void
  (e: 'delete'): void
}>()
</script>

<style scoped src="./styles/node-shared.css"></style>
<style scoped>
.node-header-icon {
  width: 16px;
  height: 16px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  opacity: 0.95;
}

.node-header-icon :deep(svg) {
  display: block;
}

.workflow-node-header--rounded {
  border-top-left-radius: 12px;
  border-top-right-radius: 12px;
}
</style>
