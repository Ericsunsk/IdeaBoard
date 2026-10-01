<template>
  <SelectViewport
    v-bind="cleanProps(props)"
    :class="cn(selectViewportVariants, props.class)"
  >
    <slot />
  </SelectViewport>
</template>

<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { SelectViewport, type SelectViewportProps } from 'reka-ui'
import { cn, cleanProps } from '@/lib/utils'
import { selectViewportVariants } from './selectVariants'

const props = withDefaults(
  defineProps<
    SelectViewportProps & {
      class?: HTMLAttributes['class']
    }
  >(),
  { class: undefined },
)
</script>
