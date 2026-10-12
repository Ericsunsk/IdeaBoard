<template>
  <div data-ui="table-wrapper" :class="cn(tableWrapperClasses, props.wrapperClass)">
    <table data-ui="table" :class="cn(tableClasses, props.class)">
      <slot />
    </table>
  </div>
</template>

<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { cn } from '@/lib/utils'
import { tableClasses, tableWrapperClasses } from './tableClasses'

const props = withDefaults(
  defineProps<{
    class?: HTMLAttributes['class']
    wrapperClass?: HTMLAttributes['class']
  }>(),
  { class: undefined, wrapperClass: undefined },
)
</script>
