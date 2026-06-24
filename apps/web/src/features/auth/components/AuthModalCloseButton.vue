<template>
  <Button
    unstyled
    type="button"
    :class="authCloseButtonClass"
    :disabled="disabled"
    aria-label="关闭"
    @click="emit('click')"
  >
    <template #icon>
      <Xmark class="size-6" aria-hidden="true" />
    </template>
  </Button>
</template>

<script setup lang="ts">
import { Xmark } from '@iconoir/vue'
import Button from 'primevue/button'
import { authCloseButtonClass } from './authModalUi'

defineProps<{
  disabled?: boolean
}>()

const emit = defineEmits<{
  (e: 'click'): void
}>()
</script>
