<template>
  <PopoverPortal>
    <PopoverContent
      :align="align"
      :side-offset="sideOffset"
      v-bind="$attrs"
      :class="
        cn(
          'z-admin-dialog min-w-[160px] rounded-panel border border-border bg-card p-1 text-foreground outline-none overlay-popover-motion',
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
import { PopoverContent, PopoverPortal, type PopoverContentProps } from 'reka-ui'
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
