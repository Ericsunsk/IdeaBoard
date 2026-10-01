<template>
  <input
    v-model="modelValue"
    :aria-invalid="props.invalid || undefined"
    :class="cn(inputVariants({ size: props.size }), props.class)"
  />
</template>

<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { type InputVariants, inputVariants } from './inputVariants'
import { cn } from '@/lib/utils'

const props = withDefaults(
  defineProps<{
    class?: HTMLAttributes['class'] | undefined
    invalid?: boolean | undefined
    size?: InputVariants['size'] | undefined
  }>(),
  {
    class: undefined,
    invalid: false,
    size: 'default',
  },
)

const modelValue = defineModel<string | number | undefined>()
</script>
