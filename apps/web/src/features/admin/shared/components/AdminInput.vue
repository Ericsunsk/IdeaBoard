<template>
  <Input
    :model-value="modelValue ?? ''"
    :aria-label="ariaLabel"
    :invalid="invalid"
    :size="size === 'small' ? 'compact' : 'default'"
    :type="type"
    :disabled="disabled"
    :placeholder="placeholder"
    :readonly="readonly"
    :class="
      cn(
        adminInputChromeClass,
        size === 'small' ? 'text-admin-meta' : 'text-admin-body',
        invalid &&
          '!border-admin-danger focus-visible:ring-admin-danger/20',
        props.class,
      )
    "
    @input="handleInput"
    @keydown.enter="$emit('enter')"
  />
</template>

<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { Input } from '@/components/ui/input'
import { cn } from '@/lib/utils'
import { adminInputChromeClass } from '../lib/uiClasses'

const props = withDefaults(
  defineProps<{
    defaultValue?: string | number | null | undefined
    modelValue?: string | number | null | undefined
    class?: HTMLAttributes['class'] | undefined
    type?: string | undefined
    disabled?: boolean | undefined
    readonly?: boolean | undefined
    placeholder?: string | undefined
    ariaLabel?: string | undefined
    invalid?: boolean | undefined
    size?: 'small' | 'default' | undefined
  }>(),
  {
    defaultValue: undefined,
    modelValue: undefined,
    class: undefined,
    type: 'text',
    disabled: false,
    readonly: false,
    placeholder: undefined,
    ariaLabel: undefined,
    invalid: false,
    size: 'default',
  },
)

const emit = defineEmits<{
  (e: 'update:modelValue', payload: string | number | null): void
  (e: 'enter'): void
}>()

function handleInput(event: Event) {
  const target = event.target as HTMLInputElement
  if (props.type === 'number') {
    const val = target.value === '' ? '' : Number(target.value)
    emit('update:modelValue', val)
  } else {
    emit('update:modelValue', target.value)
  }
}
</script>
