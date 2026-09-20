<template>
  <div
    data-ui="skeleton"
    :class="cn('animate-pulse rounded-md bg-muted', props.class)"
    :style="skeletonStyle"
  />
</template>

<script setup lang="ts">
import { computed, type CSSProperties, type HTMLAttributes } from 'vue'
import { cn } from '@/lib/utils'

interface SkeletonProps {
  class?: HTMLAttributes['class']
  width?: string | undefined
  height?: string | undefined
}

const props = withDefaults(defineProps<SkeletonProps>(), {
  class: undefined,
  width: undefined,
  height: undefined,
})

const skeletonStyle = computed<CSSProperties | undefined>(() => {
  if (!props.width && !props.height) return undefined
  return {
    ...(props.width ? { width: props.width } : {}),
    ...(props.height ? { height: props.height } : {}),
  }
})
</script>
