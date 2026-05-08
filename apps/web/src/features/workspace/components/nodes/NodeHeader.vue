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
    <button class="node-delete-btn" title="删除" @mousedown.stop @click.stop="emit('delete')">
      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
        <line x1="18" y1="6" x2="6" y2="18" />
        <line x1="6" y1="6" x2="18" y2="18" />
      </svg>
    </button>
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

<style scoped src="./node-shared.css"></style>
<style scoped>
.node-header-icon {
  display: inline-flex;
  margin-right: 6px;
  opacity: 0.8;
}

.node-header-icon :deep(svg) {
  display: block;
}

.workflow-node-header--rounded {
  border-top-left-radius: 12px;
  border-top-right-radius: 12px;
}
</style>
