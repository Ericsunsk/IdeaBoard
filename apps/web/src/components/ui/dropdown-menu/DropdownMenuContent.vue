<template>
  <DropdownMenuPortal>
    <DropdownMenuContent
      v-bind="cleanProps(props)"
      :class="
        cn(
          'z-popover min-w-32 overflow-hidden rounded-panel border border-border bg-popover p-1 text-popover-foreground outline-none overlay-popover-motion',
          props.class,
        )
      "
      @escape-key-down="emits('escapeKeyDown', $event)"
      @pointer-down-outside="emits('pointerDownOutside', $event)"
      @focus-outside="emits('focusOutside', $event)"
      @interact-outside="emits('interactOutside', $event)"
      @close-auto-focus="emits('closeAutoFocus', $event)"
    >
      <slot />
    </DropdownMenuContent>
  </DropdownMenuPortal>
</template>

<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import {
  DropdownMenuContent,
  type DropdownMenuContentEmits,
  type DropdownMenuContentProps,
  DropdownMenuPortal,
} from 'reka-ui'
import { cn, cleanProps } from '@/lib/utils'

const props = withDefaults(
  defineProps<
    DropdownMenuContentProps & {
      class?: HTMLAttributes['class']
    }
  >(),
  {
    sideOffset: 4,
    class: undefined,
  },
)

const emits = defineEmits<DropdownMenuContentEmits>()

defineOptions({
  inheritAttrs: false,
})
</script>
