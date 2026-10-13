<template>
  <label
    :for="formItemId"
    :class="
      cn(
        'text-sm font-medium leading-none peer-disabled:cursor-not-allowed peer-disabled:opacity-50',
        error && 'text-destructive',
        props.class,
      )
    "
  >
    <slot />
  </label>
</template>

<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { useFormField } from './context'
import { cn } from '@/lib/utils'

const props = defineProps<{
  class?: HTMLAttributes['class']
}>()

const { formItemId, error } = useFormField()
</script>
