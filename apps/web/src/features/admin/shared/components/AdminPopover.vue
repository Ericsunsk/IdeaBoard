<template>
  <PopoverRoot
    :open="open"
    :default-open="defaultOpen"
    :modal="modal"
    @update:open="(val) => emit('update:open', val)"
  >
    <slot />
  </PopoverRoot>
</template>

<script setup lang="ts">
import { PopoverRoot, type PopoverRootProps } from 'reka-ui'

defineProps<PopoverRootProps>()
const emit = defineEmits<{
  'update:open': [value: boolean]
}>()
</script>
