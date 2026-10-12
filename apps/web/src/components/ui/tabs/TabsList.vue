<template>
  <TabsList v-bind="forwarded" :class="cn(tabsListVariants, props.class)">
    <slot />
  </TabsList>
</template>

<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { computed } from 'vue'
import { TabsList, type TabsListProps } from 'reka-ui'
import { cn, cleanProps } from '@/lib/utils'
import { tabsListVariants } from './tabsVariants'

const props = withDefaults(defineProps<TabsListProps & { class?: HTMLAttributes['class'] }>(), {
  class: undefined,
})

// `class` is a local skin prop, not a reka prop. Forwarding it would render
// it as a stray attribute on the tablist element.
const forwarded = computed<TabsListProps>(() => {
  const { class: _class, ...rest } = props
  return cleanProps(rest) as TabsListProps
})
</script>
