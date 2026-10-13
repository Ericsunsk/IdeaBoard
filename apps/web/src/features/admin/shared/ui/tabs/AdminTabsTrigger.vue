<template>
  <TabsTrigger
    :value="value"
    :disabled="disabled"
    v-bind="$attrs"
    :class="
      cn(
        // h-7 (28px) perfectly fits the h-9 (36px) rail with p-1 (4px padding all around).
        // hit-target-32-inline preserves the repo 32px touch-target floor along the block axis.
        'inline-flex h-7 items-center justify-center gap-1.5 whitespace-nowrap rounded-control px-3 text-admin-caption font-medium text-muted-foreground transition-colors cursor-pointer select-none focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-1 disabled:pointer-events-none disabled:opacity-50 hover:text-foreground data-[state=active]:bg-card data-[state=active]:font-semibold data-[state=active]:text-foreground data-[state=active]:shadow-xs hit-target-32-inline',
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
