<template>
  <button
    type="button"
    class="node-icon-btn"
    :class="[{ active }, `tone-${tone}`]"
    :style="{ width: `${size}px`, height: `${size}px` }"
    :disabled="disabled"
    :title="title"
    @click="emit('click', $event)"
  >
    <slot />
  </button>
</template>

<script setup lang="ts">
withDefaults(
  defineProps<{
    tone?: 'tool' | 'accent'
    active?: boolean
    disabled?: boolean
    size?: number
    title?: string
  }>(),
  {
    tone: 'tool',
    active: false,
    disabled: false,
    size: 28,
    title: '',
  },
)

const emit = defineEmits<{
  (e: 'click', event: MouseEvent): void
}>()
</script>

<style scoped>
.node-icon-btn {
  border: var(--node-tool-border-width, 1.2px) solid #d6dcec;
  background: #ffffff;
  color: #5f6877;
  border-radius: 8px;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 0;
  cursor: pointer;
  transition: all 0.2s ease;
}

.node-icon-btn.tone-tool:hover:not(:disabled) {
  border-color: #c4b7f4;
  color: #6757c8;
}

.node-icon-btn.tone-tool.active {
  border-color: rgba(122, 107, 213, 0.4);
  background: rgba(122, 107, 213, 0.12);
  color: #6757c8;
}

.node-icon-btn.tone-accent {
  color: #6f61c6;
}

.node-icon-btn.tone-accent:hover:not(:disabled) {
  border-color: #b9a8f0;
  background: #f5f2ff;
  color: #5f4fd0;
}

.node-icon-btn:disabled {
  cursor: not-allowed;
  opacity: 0.45;
}
</style>
