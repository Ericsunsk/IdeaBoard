<template>
  <TabsTrigger
    :value="value"
    :disabled="disabled"
    v-bind="$attrs"
    :class="
      cn(
        // min-h-8 (32px) is the repo touch-target floor; the surrounding
        // AdminTabsList is h-9, so the taller trigger still fits its rail.
        'inline-flex min-h-8 items-center justify-center gap-1.5 whitespace-nowrap rounded-md px-3 text-admin-caption font-medium text-admin-text-2 transition-colors cursor-pointer select-none focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-admin-accent focus-visible:ring-offset-1 disabled:pointer-events-none disabled:opacity-50 hover:text-admin-text-1 data-[state=active]:bg-white data-[state=active]:font-semibold data-[state=active]:text-admin-text-1 data-[state=active]:shadow-xs',
        props.class,
      )
    "
  >
    <slot />
  </TabsTrigger>
</template>

<script setup lang="ts">
import { type HTMLAttributes } from 'vue'
import { TabsTrigger } from 'reka-ui'
import { cn } from '@/lib/utils'

const props = defineProps<{
  value: string
  disabled?: boolean
  class?: HTMLAttributes['class']
}>()

defineOptions({
  inheritAttrs: false,
})
</script>
