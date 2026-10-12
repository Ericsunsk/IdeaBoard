<template>
  <tr data-ui="table-row" :class="cn(tableRowClasses, props.class)">
    <slot />
  </tr>
</template>

<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { cn } from '@/lib/utils'
import { tableRowClasses } from './tableClasses'

/**
 * Native `<tr>`: every non-prop attribute (`tabindex`, `aria-*`, `data-*`)
 * falls through to the row, which is how a row opts into keyboard
 * reachability. Rows and cells stay out of the tab order by default.
 */
const props = withDefaults(defineProps<{ class?: HTMLAttributes['class'] }>(), {
  class: undefined,
})
</script>
