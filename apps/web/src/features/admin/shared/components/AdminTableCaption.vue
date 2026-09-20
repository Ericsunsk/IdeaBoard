<template>
  <caption :class="cn('mt-4 text-xs text-admin-text-3', props.class)">
    <slot />
  </caption>
</template>

<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { cn } from '@/lib/utils'

const props = defineProps<{
  class?: HTMLAttributes['class'] | undefined
}>()
</script>
