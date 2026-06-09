<template>
  <Button
    type="button"
    rounded
    text
    severity="secondary"
    class="!absolute !top-6 !right-6 !size-8 !p-0 !text-[#1d1d1f] !shadow-none hover:!bg-black/10 disabled:!cursor-not-allowed disabled:!opacity-50"
    :disabled="disabled"
    aria-label="关闭"
    @click="emit('click')"
  >
    <template #icon>
      <Xmark class="size-6 stroke-2" aria-hidden="true" />
    </template>
  </Button>
</template>

<script setup lang="ts">
import { Xmark } from '@iconoir/vue'
import Button from 'primevue/button'

defineProps<{
  disabled?: boolean
}>()

const emit = defineEmits<{
  (e: 'click'): void
}>()
</script>
