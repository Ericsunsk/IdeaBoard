<template>
  <button
    type="button"
    class="node-action-btn"
    :class="[`variant-${variant}`, { active, full, 'push-right': pushRight }]"
    :disabled="disabled"
    :title="title"
    @click="emit('click', $event)"
  >
    <slot />
  </button>
</template>

<script setup lang="ts">
withDefaults(defineProps<{
  variant?: 'stage' | 'stage-ghost' | 'tool' | 'chip' | 'primary'
  active?: boolean
  disabled?: boolean
  full?: boolean
  pushRight?: boolean
  title?: string
}>(), {
  variant: 'tool',
  active: false,
  disabled: false,
  full: false,
  pushRight: false,
  title: '',
})

const emit = defineEmits<{
  (e: 'click', event: MouseEvent): void
}>()
</script>

<style scoped>
.node-action-btn {
  border: 1px solid #d6dcec;
  background: #ffffff;
  color: #5f6877;
  border-radius: 8px;
  min-height: 28px;
  padding: 0 9px;
  font-size: 11px;
  font-weight: 600;
  cursor: pointer;
  transition: all 0.2s ease;
}

.node-action-btn.full {
  width: 100%;
}

.node-action-btn.push-right {
  margin-left: auto;
}

.node-action-btn.variant-stage {
  border: none;
  border-radius: 8px;
  background: rgba(17, 24, 39, 0.74);
  color: #f9fafb;
  min-height: 26px;
  padding: 0 10px;
  font-weight: 500;
}

.node-action-btn.variant-stage:hover:not(:disabled) {
  background: rgba(17, 24, 39, 0.9);
}

.node-action-btn.variant-stage-ghost {
  border: none;
  border-radius: 8px;
  background: rgba(75, 85, 99, 0.72);
  color: #f9fafb;
  min-height: 26px;
  padding: 0 10px;
  font-weight: 500;
}

.node-action-btn.variant-stage-ghost:hover:not(:disabled) {
  background: rgba(75, 85, 99, 0.88);
}

.node-action-btn.variant-tool:hover:not(:disabled) {
  border-color: #c4b7f4;
  color: #6757c8;
}

.node-action-btn.variant-tool.active {
  border-color: rgba(122, 107, 213, 0.4);
  background: rgba(122, 107, 213, 0.12);
  color: #6757c8;
}

.node-action-btn.variant-chip {
  min-height: 24px;
  padding: 0 8px;
}

.node-action-btn.variant-chip:hover:not(:disabled) {
  border-color: #c4b7f4;
  color: #6757c8;
}

.node-action-btn.variant-chip.active {
  border-color: rgba(122, 107, 213, 0.4);
  background: rgba(122, 107, 213, 0.12);
  color: #6757c8;
}

.node-action-btn.variant-primary {
  min-height: 34px;
  border-color: #9278e6;
  border-radius: 10px;
  background: #9278e6;
  color: #ffffff;
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0.01em;
  padding: 0 12px;
}

.node-action-btn.variant-primary:hover:not(:disabled) {
  background: #7f68db;
  border-color: #7f68db;
}

.node-action-btn:disabled {
  cursor: not-allowed;
  opacity: 0.45;
}
</style>
