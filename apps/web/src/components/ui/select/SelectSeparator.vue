<template>
  <SelectSeparator
    v-bind="cleanProps(props)"
    :class="cn('-mx-1 my-1 h-px bg-border', props.class)"
  />
</template>

<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { SelectSeparator, type SelectSeparatorProps } from 'reka-ui'
import { cn, cleanProps } from '@/lib/utils'

const props = withDefaults(
  defineProps<
    SelectSeparatorProps & {
      class?: HTMLAttributes['class']
    }
  >(),
  { class: undefined },
)
</script>
