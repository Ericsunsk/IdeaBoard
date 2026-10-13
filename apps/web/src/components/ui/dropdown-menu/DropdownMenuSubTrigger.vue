<template>
  <DropdownMenuSubTrigger
    v-bind="forwardedProps"
    :class="
      cn(
        'flex cursor-default select-none items-center rounded-control px-2 py-1.5 text-sm outline-none transition-colors duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] focus:bg-accent data-[state=open]:bg-accent',
        inset && 'pl-8',
        props.class,
      )
    "
  >
    <slot />
    <NavArrowRight class="ml-auto h-4 w-4 shrink-0" />
  </DropdownMenuSubTrigger>
</template>

<script setup lang="ts">
import { computed, type HTMLAttributes } from 'vue'
import { DropdownMenuSubTrigger, type DropdownMenuSubTriggerProps } from 'reka-ui'
import { NavArrowRight } from '@iconoir/vue'
import { cn, cleanProps } from '@/lib/utils'

const props = withDefaults(
  defineProps<
    DropdownMenuSubTriggerProps & {
      class?: HTMLAttributes['class']
      inset?: boolean
    }
  >(),
  {
    class: undefined,
    inset: false,
  },
)

const forwardedProps = computed(() => {
  const { class: _, inset: _i, ...rest } = props
  return cleanProps(rest)
})
</script>
