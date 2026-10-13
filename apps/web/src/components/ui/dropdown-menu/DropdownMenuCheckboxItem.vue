<template>
  <DropdownMenuCheckboxItem
    v-bind="cleanProps(props)"
    :class="
      cn(
        'relative flex cursor-default select-none items-center rounded-control py-1.5 pl-8 pr-2 text-sm outline-none transition-colors duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] focus:bg-accent focus:text-accent-foreground data-[disabled]:pointer-events-none data-[disabled]:opacity-50',
        props.class,
      )
    "
    @update:model-value="emits('update:modelValue', $event)"
    @select="emits('select', $event)"
  >
    <span class="absolute left-2 flex h-3.5 w-3.5 items-center justify-center">
      <DropdownMenuItemIndicator>
        <Check class="h-4 w-4" />
      </DropdownMenuItemIndicator>
    </span>
    <slot />
  </DropdownMenuCheckboxItem>
</template>

<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import {
  DropdownMenuCheckboxItem,
  type DropdownMenuCheckboxItemEmits,
  type DropdownMenuCheckboxItemProps,
  DropdownMenuItemIndicator,
} from 'reka-ui'
import { Check } from '@iconoir/vue'
import { cn, cleanProps } from '@/lib/utils'

const props = defineProps<
  DropdownMenuCheckboxItemProps & {
    class?: HTMLAttributes['class']
  }
>()
const emits = defineEmits<DropdownMenuCheckboxItemEmits>()
</script>
