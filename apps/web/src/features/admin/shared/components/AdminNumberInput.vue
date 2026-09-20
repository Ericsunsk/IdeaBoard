<template>
  <input
    :id="inputId || id"
    ref="inputRef"
    type="text"
    role="spinbutton"
    :aria-valuenow="modelValue ?? undefined"
    :aria-valuemin="min"
    :aria-valuemax="max"
    :aria-invalid="invalid || undefined"
    :value="displayValue"
    :class="
      cn(
        adminInputChromeClass,
        'h-9 px-3 py-1.5 text-admin-body',
        size === 'small' && 'h-8 px-2.5 text-admin-meta',
        invalid && 'border-admin-danger focus-visible:border-admin-danger focus-visible:ring-admin-danger/20',
        props.class,
      )
    "
    :disabled="disabled"
    :readonly="readonly"
    :placeholder="placeholder"
    @focus="isEditing = true"
    @blur="isEditing = false"
    @input="handleInput"
    @keydown.enter="$emit('enter')"
  />
</template>

<script setup lang="ts">
import { computed, ref, type HTMLAttributes } from 'vue'
import { cn } from '@/lib/utils'
import { adminInputChromeClass } from '../lib/uiClasses'

const props = withDefaults(
  defineProps<{
    modelValue?: number | null | undefined
    defaultValue?: number | null | undefined
    inputId?: string | undefined
    id?: string | undefined
    min?: number | undefined
    max?: number | undefined
    step?: number | undefined
    suffix?: string | undefined
    useGrouping?: boolean | undefined
    disabled?: boolean | undefined
    readonly?: boolean | undefined
    placeholder?: string | undefined
    invalid?: boolean | undefined
    size?: 'small' | 'default' | undefined
    fluid?: boolean | undefined
    class?: HTMLAttributes['class'] | undefined
  }>(),
  {
    modelValue: undefined,
    defaultValue: undefined,
    inputId: undefined,
    id: undefined,
    min: undefined,
    max: undefined,
    step: undefined,
    suffix: undefined,
    useGrouping: undefined,
    disabled: false,
    readonly: false,
    placeholder: undefined,
    invalid: false,
    size: 'default',
    fluid: undefined,
    class: undefined,
  },
)

const emit = defineEmits<{
  (e: 'update:modelValue', payload: number | null): void
  (e: 'enter'): void
}>()

const inputRef = ref<HTMLInputElement | null>(null)
const isEditing = ref(false)

const displayValue = computed(() => {
  if (props.modelValue === null || props.modelValue === undefined || props.modelValue === ('' as unknown as number)) {
    return ''
  }
  if (props.suffix && !isEditing.value) {
    return `${props.modelValue}${props.suffix}`
  }
  return String(props.modelValue)
})

function handleInput(event: Event) {
  const target = event.target as HTMLInputElement
  const raw = target.value.trim()
  if (!raw) {
    emit('update:modelValue', null)
    return
  }

  // Extract leading or matching number part
  const match = raw.match(/-?\d+(\.\d+)?/)
  if (!match) {
    emit('update:modelValue', null)
    return
  }

  let num = Number(match[0])
  if (Number.isNaN(num)) {
    emit('update:modelValue', null)
    return
  }

  if (props.min !== undefined && num < props.min) num = props.min
  if (props.max !== undefined && num > props.max) num = props.max

  emit('update:modelValue', num)
}
</script>
