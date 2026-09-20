<template>
  <form :class="props.class" @submit.prevent="handleSubmit">
    <slot />
  </form>
</template>

<script setup lang="ts">
import type { HTMLAttributes } from 'vue'

const props = defineProps<{
  class?: HTMLAttributes['class']
}>()

const emit = defineEmits<{
  submit: [e: Event]
}>()

function handleSubmit(e: Event) {
  emit('submit', e)
}
</script>
