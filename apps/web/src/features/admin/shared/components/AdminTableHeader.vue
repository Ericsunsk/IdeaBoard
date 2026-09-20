<template>
  <thead :class="cn('[&_tr]:border-b [&_tr]:border-admin-border', props.class)">
    <slot />
  </thead>
</template>

<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { cn } from '@/lib/utils'

const props = defineProps<{
  class?: HTMLAttributes['class'] | undefined
}>()
</script>
