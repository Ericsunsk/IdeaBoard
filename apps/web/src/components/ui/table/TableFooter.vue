<template>
  <tfoot data-ui="table-footer" :class="cn(tableFooterClasses, props.class)">
    <slot />
  </tfoot>
</template>

<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { cn } from '@/lib/utils'
import { tableFooterClasses } from './tableClasses'

const props = withDefaults(defineProps<{ class?: HTMLAttributes['class'] }>(), {
  class: undefined,
})
</script>
