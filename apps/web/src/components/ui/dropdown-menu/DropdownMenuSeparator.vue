<template>
  <DropdownMenuSeparator
    v-bind="cleanProps(props)"
    :class="cn('-mx-1 my-1 h-px bg-muted', props.class)"
  />
</template>

<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import {
  DropdownMenuSeparator,
  type DropdownMenuSeparatorProps,
} from 'reka-ui'
import { cn, cleanProps } from '@/lib/utils'

const props = defineProps<
  DropdownMenuSeparatorProps & {
    class?: HTMLAttributes['class']
  }
>()
</script>
