<template>
  <div :class="cn('space-y-2', props.class)">
    <slot />
  </div>
</template>

<script setup lang="ts">
import { provide, useId, type HTMLAttributes } from 'vue'
import { FORM_ITEM_CONTEXT_KEY } from './context'
import { cn } from '@/lib/utils'

const props = defineProps<{
  class?: HTMLAttributes['class']
}>()

const id = useId()
provide(FORM_ITEM_CONTEXT_KEY, { id })
</script>
