<template>
  <SelectItem
    v-bind="forwarded"
    :class="cn(selectItemVariants, inset && 'pl-8', props.class)"
    @select="emits('select', $event)"
  >
    <slot />
  </SelectItem>
</template>

<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { SelectItem, type SelectItemEmits, type SelectItemProps } from 'reka-ui'
import { computed } from 'vue'
import { cn, cleanProps } from '@/lib/utils'
import { selectItemVariants } from './selectVariants'

// `value` is required by reka, so it is forwarded explicitly rather than left to
// the caller, while every optional prop still travels through cleanProps. This
// keeps the wrapper transparent: callers pass exactly the reka props.
const props = withDefaults(
  defineProps<
    SelectItemProps & {
      class?: HTMLAttributes['class']
      inset?: boolean
    }
  >(),
  { class: undefined, inset: false },
)
const emits = defineEmits<SelectItemEmits>()

// cleanProps drops undefined entries, which is exactly the behaviour reka wants
// for optional props, but it erases the required `value` from the type. The cast
// restores it: `value` is declared non-optional on the wrapper props.
const forwarded = computed<SelectItemProps>(() => {
  const { class: _class, inset: _inset, ...rest } = props
  return cleanProps(rest) as unknown as SelectItemProps
})
</script>
