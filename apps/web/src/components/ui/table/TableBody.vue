<template>
  <tbody data-ui="table-body" :class="cn(tableBodyClasses, props.class)">
    <slot />
  </tbody>
</template>

<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { cn } from '@/lib/utils'
import { tableBodyClasses } from './tableClasses'

const props = withDefaults(defineProps<{ class?: HTMLAttributes['class'] }>(), {
  class: undefined,
})
</script>
