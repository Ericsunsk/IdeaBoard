<template>
  <textarea
    :id="id"
    ref="innerRef"
    :value="modelValue ?? ''"
    :aria-label="ariaLabel"
    :class="
      cn(
        adminInputChromeClass,
        'min-h-[80px] px-3 py-2 text-admin-body',
        autoGrow && 'resize-none',
        props.class,
      )
    "
    :disabled="disabled"
    :placeholder="placeholder"
    :rows="rows"
    :readonly="readonly"
    @input="emit('update:modelValue', ($event.target as HTMLTextAreaElement).value)"
  />
</template>

<script setup lang="ts">
import { nextTick, onMounted, ref, watch } from 'vue'
import type { HTMLAttributes } from 'vue'
import { cn } from '../lib/classNames'
import { adminInputChromeClass } from '../lib/uiClasses'

const props = withDefaults(
  defineProps<{
    defaultValue?: string | null | undefined
    modelValue?: string | null | undefined
    class?: HTMLAttributes['class'] | undefined
    disabled?: boolean | undefined
    readonly?: boolean | undefined
    placeholder?: string | undefined
    ariaLabel?: string | undefined
    id?: string | undefined
    rows?: number | undefined
    autoGrow?: boolean | undefined
  }>(),
  {
    defaultValue: undefined,
    modelValue: undefined,
    class: undefined,
    disabled: false,
    readonly: false,
    placeholder: undefined,
    ariaLabel: undefined,
    id: undefined,
    rows: 3,
    autoGrow: false,
  },
)

const emit = defineEmits<{
  (e: 'update:modelValue', payload: string): void
}>()

const innerRef = ref<HTMLTextAreaElement | null>(null)

function adjustHeight() {
  const element = innerRef.value
  if (!element) return
  element.style.height = 'auto'
  element.style.height = `${element.scrollHeight}px`
}

onMounted(() => {
  if (props.autoGrow) void nextTick(adjustHeight)
})

watch(
  () => props.modelValue,
  () => {
    if (props.autoGrow) void nextTick(adjustHeight)
  },
)
</script>
