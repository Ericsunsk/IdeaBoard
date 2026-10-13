<template>
  <div data-ui="table-wrapper" :class="cn('relative w-full overflow-auto', props.wrapperClass)">
    <table data-ui="table" :class="cn('w-full caption-bottom text-sm', props.class)">
      <slot />
    </table>
  </div>
</template>

<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { cn } from '@/lib/utils'

const props = withDefaults(
  defineProps<{
    class?: HTMLAttributes['class']
    wrapperClass?: HTMLAttributes['class']
  }>(),
  { class: undefined, wrapperClass: undefined },
)
</script>
