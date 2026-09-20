<template>
  <Primitive
    :as="as"
    :as-child="asChild"
    :class="cn(buttonVariants({ variant: props.variant, size: props.size }), props.class)"
  >
    <Spinner v-if="props.loading" class="size-4 shrink-0 mr-2" aria-hidden="true" />
    <slot />
  </Primitive>
</template>

<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { Primitive, type PrimitiveProps } from 'reka-ui'
import { type ButtonVariants, buttonVariants } from './buttonVariants'
import { Spinner } from '@/components/ui/spinner'
import { cn } from '@/lib/utils'

interface Props extends PrimitiveProps {
  variant?: ButtonVariants['variant'] | undefined
  size?: ButtonVariants['size'] | undefined
  class?: HTMLAttributes['class'] | undefined
  loading?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  as: 'button',
  variant: 'default',
  size: 'default',
  class: undefined,
  loading: false,
})
</script>
