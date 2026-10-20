<template>
  <DialogOverlay
    :class="cn('fixed inset-0 z-admin-dialog bg-dialog-mask overlay-mask-motion', props.class)"
    v-bind="cleanProps(props)"
  >
    <slot />
  </DialogOverlay>
</template>

<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { DialogOverlay, type DialogOverlayProps } from 'reka-ui'
import { cn, cleanProps } from '@/lib/utils'

const props = defineProps<DialogOverlayProps & { class?: HTMLAttributes['class'] }>()
</script>
