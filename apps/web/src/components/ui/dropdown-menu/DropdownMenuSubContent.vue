<template>
  <DropdownMenuPortal>
    <DropdownMenuSubContent
      v-bind="cleanProps(props)"
      :class="
        cn(
          'z-50 min-w-32 overflow-hidden rounded-panel border border-border bg-popover p-1 text-popover-foreground shadow-lg outline-none overlay-popover-motion',
          props.class,
        )
      "
      @escape-key-down="emits('escapeKeyDown', $event)"
      @pointer-down-outside="emits('pointerDownOutside', $event)"
      @focus-outside="emits('focusOutside', $event)"
      @interact-outside="emits('interactOutside', $event)"
      @entry-focus="emits('entryFocus', $event)"
      @open-auto-focus="emits('openAutoFocus', $event)"
      @close-auto-focus="emits('closeAutoFocus', $event)"
    >
      <slot />
    </DropdownMenuSubContent>
  </DropdownMenuPortal>
</template>

<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import {
  DropdownMenuPortal,
  DropdownMenuSubContent,
  type DropdownMenuSubContentEmits,
  type DropdownMenuSubContentProps,
} from 'reka-ui'
import { cn, cleanProps } from '@/lib/utils'

const props = withDefaults(
  defineProps<
    DropdownMenuSubContentProps & {
      class?: HTMLAttributes['class']
    }
  >(),
  {
    class: undefined,
  },
)
const emits = defineEmits<DropdownMenuSubContentEmits>()

defineOptions({
  inheritAttrs: false,
})
</script>
