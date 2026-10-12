<template>
  <TabsContent v-bind="forwarded" :class="cn(tabsContentVariants, props.class)">
    <slot />
  </TabsContent>
</template>

<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { computed } from 'vue'
import { TabsContent, type TabsContentProps } from 'reka-ui'
import { cn, cleanProps } from '@/lib/utils'
import { tabsContentVariants } from './tabsVariants'

const props = withDefaults(defineProps<TabsContentProps & { class?: HTMLAttributes['class'] }>(), {
  class: undefined,
})

const forwarded = computed<TabsContentProps>(() => {
  const { class: _class, ...rest } = props
  return cleanProps(rest) as unknown as TabsContentProps
})
</script>
