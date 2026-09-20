<template>
  <component
    :is="as"
    :class="cn('font-semibold leading-none tracking-tight', props.class)"
  >
    <slot />
  </component>
</template>

<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { cn } from '@/lib/utils'

interface Props {
  as?: string
  class?: HTMLAttributes['class']
}

const props = withDefaults(defineProps<Props>(), {
  as: 'h3',
  class: undefined,
})
</script>
