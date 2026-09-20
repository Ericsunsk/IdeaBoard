<template>
  <SwitchRoot
    :model-value="modelValue"
    :disabled="disabled"
    :class="
      cn(
        'peer inline-flex h-5 w-9 shrink-0 cursor-pointer items-center rounded-full border-2 border-transparent transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-admin-accent/20 disabled:cursor-not-allowed disabled:opacity-50 data-[state=checked]:bg-admin-accent data-[state=unchecked]:bg-admin-border',
        props.class,
      )
    "
    @update:model-value="(val: boolean) => emit('update:modelValue', val)"
  >
    <SwitchThumb
      :class="
        cn(
          'pointer-events-none block size-4 rounded-full bg-white shadow-lg ring-0 transition-transform data-[state=checked]:translate-x-4 data-[state=unchecked]:translate-x-0',
        )
      "
    />
  </SwitchRoot>
</template>

<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { SwitchRoot, SwitchThumb } from 'reka-ui'
import { cn } from '../lib/classNames'

const props = withDefaults(
  defineProps<{
    modelValue?: boolean
    class?: HTMLAttributes['class']
    disabled?: boolean
  }>(),
  {
    modelValue: false,
    class: undefined,
    disabled: false,
  },
)

const emit = defineEmits<{
  (e: 'update:modelValue', value: boolean): void
}>()
</script>
