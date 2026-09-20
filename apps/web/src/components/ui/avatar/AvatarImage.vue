<template>
  <AvatarImage
    :src="props.src"
    :alt="props.alt"
    :class="cn('aspect-square size-full object-cover', props.class)"
  />
</template>

<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { AvatarImage } from 'reka-ui'
import { cn } from '@/lib/utils'

const props = withDefaults(
  defineProps<{
    src: string
    alt?: string | undefined
    class?: HTMLAttributes['class']
  }>(),
  {
    alt: undefined,
    class: undefined,
  },
)
</script>
