<template>
  <SelectLabel
    v-bind="cleanProps(props)"
    :class="cn('px-2 py-1.5 text-xs text-muted-foreground', props.class)"
  >
    <slot />
  </SelectLabel>
</template>

<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { SelectLabel, type SelectLabelProps } from 'reka-ui'
import { cn, cleanProps } from '@/lib/utils'

const props = withDefaults(
  defineProps<
    SelectLabelProps & {
      class?: HTMLAttributes['class']
    }
  >(),
  { class: undefined },
)
</script>
