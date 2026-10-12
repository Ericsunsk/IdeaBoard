<template>
  <TabsTrigger v-bind="forwarded" :class="cn(tabsTriggerVariants, props.class)">
    <slot />
  </TabsTrigger>
</template>

<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { computed } from 'vue'
import { TabsTrigger, type TabsTriggerProps } from 'reka-ui'
import { cn, cleanProps } from '@/lib/utils'
import { tabsTriggerVariants } from './tabsVariants'

const props = withDefaults(defineProps<TabsTriggerProps & { class?: HTMLAttributes['class'] }>(), {
  class: undefined,
})

// `value` is required by reka, so it travels through with every other prop;
// only the local `class` skin is stripped before forwarding.
const forwarded = computed<TabsTriggerProps>(() => {
  const { class: _class, ...rest } = props
  return cleanProps(rest) as unknown as TabsTriggerProps
})
</script>
