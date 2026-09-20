<template>
  <TabsList
    v-bind="$attrs"
    :class="
      cn(
        'inline-flex h-9 items-center justify-start rounded-lg border border-admin-border-soft bg-admin-surface-muted p-1 text-admin-text-3',
        props.class,
      )
    "
  >
    <slot />
  </TabsList>
</template>

<script setup lang="ts">
import { type HTMLAttributes } from 'vue'
import { TabsList } from 'reka-ui'
import { cn } from '../lib/classNames'

const props = defineProps<{
  class?: HTMLAttributes['class']
}>()

defineOptions({
  inheritAttrs: false,
})

</script>
