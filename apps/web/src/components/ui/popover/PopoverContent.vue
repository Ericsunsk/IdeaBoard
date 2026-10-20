<template>
  <PopoverPortal>
    <PopoverContent
      v-bind="{ ...$attrs, ...cleanProps(props) }"
      :class="
        cn(
          'z-popover w-72 rounded-panel border border-border bg-popover p-4 text-popover-foreground shadow-admin-popover outline-none overlay-popover-motion',
          props.class,
        )
      "
      @escape-key-down="emits('escapeKeyDown', $event)"
      @pointer-down-outside="emits('pointerDownOutside', $event)"
      @focus-outside="emits('focusOutside', $event)"
      @interact-outside="emits('interactOutside', $event)"
      @open-auto-focus="emits('openAutoFocus', $event)"
      @close-auto-focus="emits('closeAutoFocus', $event)"
    >
      <slot />
    </PopoverContent>
  </PopoverPortal>
</template>

<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import {
  PopoverContent,
  type PopoverContentEmits,
  type PopoverContentProps,
  PopoverPortal,
} from 'reka-ui'
import { cn, cleanProps } from '@/lib/utils'

const props = withDefaults(
  defineProps<
    PopoverContentProps & {
      class?: HTMLAttributes['class']
    }
  >(),
  {
    align: 'center',
    sideOffset: 4,
    class: undefined,
  },
)
const emits = defineEmits<PopoverContentEmits>()

defineOptions({
  inheritAttrs: false,
})
</script>
