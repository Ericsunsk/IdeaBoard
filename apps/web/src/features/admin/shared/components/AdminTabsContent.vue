<template>
  <TabsContent
    :value="value"
    v-bind="$attrs"
    :class="
      cn(
        'mt-3.5 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-1',
        props.class,
      )
    "
  >
    <slot />
  </TabsContent>
</template>

<script setup lang="ts">
import { type HTMLAttributes } from 'vue'
import { TabsContent } from 'reka-ui'
import { cn } from '@/lib/utils'

const props = defineProps<{
  value: string
  class?: HTMLAttributes['class']
}>()

defineOptions({
  inheritAttrs: false,
})
</script>
