<template>
  <Keyframes
    v-if="variant === 'fusion'"
    class="node-empty-state-icon node-empty-state-icon--fusion"
    aria-hidden="true"
  />
  <MediaImage
    v-else
    class="node-empty-state-icon node-empty-state-icon--image"
    aria-hidden="true"
  />
</template>

<script setup lang="ts">
import { Keyframes, MediaImage } from '@iconoir/vue'

withDefaults(
  defineProps<{
    variant?: 'image' | 'fusion'
  }>(),
  {
    variant: 'image',
  },
)
</script>
