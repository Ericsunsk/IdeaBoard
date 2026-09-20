<template>
  <PopoverPortal>
    <PopoverContent
      :align="align"
      :side-offset="sideOffset"
      v-bind="$attrs"
      :class="
        cn(
          'z-50 min-w-[160px] rounded-xl border border-admin-border bg-admin-card-strong p-1 text-admin-text-1 outline-none data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0 data-[state=closed]:zoom-out-95 data-[state=open]:zoom-in-95 data-[side=bottom]:slide-in-from-top-2 data-[side=top]:slide-in-from-bottom-2',
          props.class,
        )
      "
    >
      <slot />
    </PopoverContent>
  </PopoverPortal>
</template>

<script setup lang="ts">
import { type HTMLAttributes } from 'vue'
import {
  PopoverContent,
  PopoverPortal,
  type PopoverContentProps,
} from 'reka-ui'
import { cn } from '@/lib/utils'

const props = withDefaults(
  defineProps<{
    align?: PopoverContentProps['align']
    sideOffset?: PopoverContentProps['sideOffset']
    class?: HTMLAttributes['class']
  }>(),
  {
    align: 'end',
    sideOffset: 4,
    class: undefined,
  },
)

defineOptions({
  inheritAttrs: false,
})
</script>
