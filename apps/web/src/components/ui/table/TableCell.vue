<template>
  <td data-ui="table-cell" :class="cn(tableCellClasses, props.class)">
    <slot />
  </td>
</template>

<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { cn } from '@/lib/utils'
import { tableCellClasses } from './tableClasses'

/**
 * Native `<td>`: non-prop attributes (`tabindex`, `aria-*`) fall through to
 * the cell for opt-in keyboard reachability. Plain cells stay out of the tab
 * order by default.
 */
const props = withDefaults(defineProps<{ class?: HTMLAttributes['class'] }>(), {
  class: undefined,
})
</script>
