<template>
  <DropdownMenuSubTrigger
    v-bind="cleanProps(props)"
    :class="
      cn(
        'flex cursor-default select-none items-center rounded-sm px-2 py-1.5 text-sm outline-none focus:bg-accent data-[state=open]:bg-accent',
        inset && 'pl-8',
        props.class,
      )
    "
  >
    <slot />
    <NavArrowRight class="ml-auto h-4 w-4" />
  </DropdownMenuSubTrigger>
</template>

<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import {
  DropdownMenuSubTrigger,
  type DropdownMenuSubTriggerProps,
} from 'reka-ui'
import { NavArrowRight } from '@iconoir/vue'
import { cn, cleanProps } from '@/lib/utils'

const props = defineProps<
  DropdownMenuSubTriggerProps & {
    class?: HTMLAttributes['class']
    inset?: boolean
  }
>()
</script>
