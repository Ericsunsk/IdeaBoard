<template>
  <thead data-ui="table-header" :class="cn(tableHeaderClasses, props.class)">
    <slot />
  </thead>
</template>

<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { cn } from '@/lib/utils'
import { tableHeaderClasses } from './tableClasses'

const props = withDefaults(defineProps<{ class?: HTMLAttributes['class'] }>(), {
  class: undefined,
})
</script>
