<template>
  <SelectPortal>
    <SelectContent
      v-bind="cleanProps(props)"
      :class="cn(selectContentVariants, props.class)"
      @escape-key-down="emits('escapeKeyDown', $event)"
      @pointer-down-outside="emits('pointerDownOutside', $event)"
      @close-auto-focus="emits('closeAutoFocus', $event)"
    >
      <slot />
    </SelectContent>
  </SelectPortal>
</template>

<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import {
  SelectContent,
  SelectPortal,
  type SelectContentEmits,
  type SelectContentProps,
} from 'reka-ui'
import { cn, cleanProps } from '@/lib/utils'
import { selectContentVariants } from './selectVariants'

const props = withDefaults(
  defineProps<
    SelectContentProps & {
      class?: HTMLAttributes['class']
    }
  >(),
  { class: undefined },
)

const emits = defineEmits<SelectContentEmits>()

defineOptions({
  inheritAttrs: false,
})
</script>
