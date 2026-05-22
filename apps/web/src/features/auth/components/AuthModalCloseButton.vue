<template>
  <button class="auth-modal-close-btn" type="button" :disabled="disabled" @click="emit('click')">
    <Xmark class="auth-modal-close-icon" />
  </button>
</template>

<script setup lang="ts">
import { Xmark } from '@iconoir/vue'

defineProps<{
  disabled?: boolean
}>()

const emit = defineEmits<{
  (e: 'click'): void
}>()
</script>

<style scoped>
.auth-modal-close-btn {
  position: absolute;
  top: 24px;
  right: 24px;
  background: rgba(0, 0, 0, 0.05);
  border: none;
  width: 32px;
  height: 32px;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  color: #1d1d1f;
  cursor: pointer;
  transition: all 0.2s;
}

.auth-modal-close-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.auth-modal-close-btn:hover:not(:disabled) {
  background: rgba(0, 0, 0, 0.1);
  transform: scale(1.1);
}

.auth-modal-close-icon {
  width: 24px;
  height: 24px;
  stroke-width: 2;
}
</style>
