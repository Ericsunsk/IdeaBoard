<template>
  <SelectTrigger v-bind="forwarded" :class="cn(selectTriggerVariants({ size }), props.class)">
    <slot />
  </SelectTrigger>
</template>

<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { computed } from 'vue'
import { SelectTrigger, type SelectTriggerProps } from 'reka-ui'
import { cn, cleanProps } from '@/lib/utils'
import { selectTriggerVariants } from './selectVariants'

const props = withDefaults(
  defineProps<
    SelectTriggerProps & {
      class?: HTMLAttributes['class']
      size?: 'default' | 'sm' | undefined
    }
  >(),
  { class: undefined, size: 'default' },
)

// `size` is a local variant, not a reka prop. Forwarding it would render it as a
// stray `size` attribute on the underlying <button> and, because reka derives the
// trigger height from attributes it understands, the list would never open.
const forwarded = computed<SelectTriggerProps>(() => {
  const { class: _class, size: _size, ...rest } = props
  return cleanProps(rest) as SelectTriggerProps
})
</script>
