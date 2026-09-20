<template>
  <p :id="formDescriptionId" :class="cn('text-[0.8rem] text-muted-foreground', props.class)">
    <slot />
  </p>
</template>

<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { useFormField } from './context'
import { cn } from '@/lib/utils'

const props = defineProps<{
  class?: HTMLAttributes['class']
}>()

const { formDescriptionId } = useFormField()
</script>
