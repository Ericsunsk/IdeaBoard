<template>
  <!-- 有 suffix/loading 时包一层相对定位容器承载尾部元素；否则保持裸 Input（零回归）。 -->
  <div v-if="$slots.suffix || loading" class="relative min-w-0">
    <Input
      v-bind="attrs"
      :model-value="modelValue ?? ''"
      :aria-label="ariaLabel"
      :aria-busy="loading || undefined"
      :invalid="invalid"
      :size="size === 'small' ? 'compact' : 'default'"
      :type="type"
      :disabled="disabled"
      :placeholder="placeholder"
      :readonly="readonly"
      :class="
        cn(
          adminInputChromeClass,
          'pr-9',
          size === 'small' ? 'text-admin-meta' : 'text-admin-body',
          invalid && '!border-destructive focus-visible:ring-destructive/20',
          props.class,
        )
      "
      @input="handleInput"
      @keydown.enter="$emit('enter')"
    />
    <span
      class="pointer-events-none absolute right-0 top-0 flex h-full items-center pr-3 text-muted-foreground"
    >
      <span v-if="loading" role="status" class="flex items-center">
        <Spinner class="size-4 shrink-0" aria-hidden="true" />
        <span class="sr-only">{{ loadingLabel }}</span>
      </span>
      <slot v-else name="suffix" />
    </span>
  </div>
  <Input
    v-else
    v-bind="attrs"
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
        invalid && '!border-destructive focus-visible:ring-destructive/20',
        props.class,
      )
    "
    @input="handleInput"
    @keydown.enter="$emit('enter')"
  />
</template>

<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { useAttrs } from 'vue'
import { Input } from '@/components/ui/input'
import { Spinner } from '@/components/ui/spinner'
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
    loading?: boolean | undefined
    loadingLabel?: string | undefined
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
    loading: false,
    loadingLabel: 'Testing…',
  },
)

const emit = defineEmits<{
  (e: 'update:modelValue', payload: string | number | null): void
  (e: 'enter'): void
}>()

defineOptions({ inheritAttrs: false })

const attrs = useAttrs()

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
