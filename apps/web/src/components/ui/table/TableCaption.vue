<template>
  <caption data-ui="table-caption" :class="cn(tableCaptionClasses, props.class)">
    <slot />
  </caption>
</template>

<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { cn } from '@/lib/utils'
import { tableCaptionClasses } from './tableClasses'

const props = withDefaults(defineProps<{ class?: HTMLAttributes['class'] }>(), {
  class: undefined,
})
</script>
