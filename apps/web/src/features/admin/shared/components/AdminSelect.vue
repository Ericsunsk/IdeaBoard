<template>
  <SelectRoot
    :model-value="modelValue != null ? String(modelValue) : ''"
    :disabled="disabled"
    @update:model-value="handleUpdate"
  >
    <SelectTrigger
      :id="id"
      v-bind="$attrs"
      :class="
        cn(
          'flex h-9 w-full items-center justify-between rounded-lg border border-admin-border bg-white px-3 py-2 text-admin-body text-admin-text-1 ring-offset-background placeholder:text-admin-text-3 focus:outline-none focus:ring-2 focus:ring-admin-accent/20 disabled:cursor-not-allowed disabled:opacity-50 [&>span]:line-clamp-1 cursor-pointer',
          size === 'small' && 'h-8 px-2.5 text-admin-meta',
          props.class,
        )
      "
    >
      <SelectValue :placeholder="placeholder">
        {{ selectedLabel || placeholder }}
      </SelectValue>
      <SelectIcon as-child>
        <NavArrowDown class="size-4 text-admin-text-3 opacity-70" />
      </SelectIcon>
    </SelectTrigger>

    <SelectPortal>
      <SelectContent
        :class="
          cn(
            'relative z-50 max-h-96 min-w-32 overflow-hidden rounded-xl border border-admin-border bg-admin-card-strong text-admin-text-1 data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0 data-[state=closed]:zoom-out-95 data-[state=open]:zoom-in-95 data-[side=bottom]:slide-in-from-top-2 data-[side=left]:slide-in-from-right-2 data-[side=right]:slide-in-from-left-2 data-[side=top]:slide-in-from-bottom-2',
          )
        "
        position="popper"
        :side-offset="4"
      >
        <SelectViewport class="p-1.5 h-[var(--reka-select-trigger-height)] w-full min-w-[var(--reka-select-trigger-width)]">
          <slot>
            <SelectItem
              v-for="opt in normalizedOptions"
              :key="String(opt.value)"
              :value="String(opt.value)"
              :class="
                cn(
                  'relative flex w-full cursor-pointer select-none items-center rounded-lg py-1.5 pr-8 pl-2 text-admin-body outline-none transition-colors focus:bg-admin-surface-muted focus:text-admin-text-1 data-[disabled]:pointer-events-none data-[disabled]:opacity-50',
                  size === 'small' && 'text-admin-meta py-1 pl-2 pr-7',
                )
              "
            >
              <span class="absolute right-2 flex size-3.5 items-center justify-center">
                <SelectItemIndicator>
                  <Check class="size-4 text-admin-accent" />
                </SelectItemIndicator>
              </span>
              <SelectItemText>{{ opt.label }}</SelectItemText>
            </SelectItem>
          </slot>
        </SelectViewport>
      </SelectContent>
    </SelectPortal>
  </SelectRoot>
</template>

<script setup lang="ts">
import { computed, type HTMLAttributes } from 'vue'
import {
  SelectContent,
  SelectIcon,
  SelectItem,
  SelectItemIndicator,
  SelectItemText,
  SelectPortal,
  SelectRoot,
  SelectTrigger,
  SelectValue,
  SelectViewport,
} from 'reka-ui'
import { Check, NavArrowDown } from '@iconoir/vue'
import { cn } from '@/lib/utils'

export type SelectOptionItem = {
  label: string
  value: string | number
}

const props = withDefaults(
  defineProps<{
    id?: string | undefined
    modelValue?: string | number | null | undefined
    options?: readonly (SelectOptionItem | Record<string, unknown>)[]
    optionLabel?: string
    optionValue?: string
    placeholder?: string
    class?: HTMLAttributes['class']
    disabled?: boolean
    size?: 'small' | 'default'
  }>(),
  {
    id: undefined,
    modelValue: '',
    options: () => [],
    optionLabel: 'label',
    optionValue: 'value',
    placeholder: '请选择...',
    class: undefined,
    disabled: false,
    size: 'default',
  },
)

const emit = defineEmits<{
  // eslint-disable-next-line @typescript-eslint/no-explicit-any -- reka passes option values through untouched; narrowing would force casts at every select call site
  (e: 'update:modelValue', value: any): void
}>()

defineOptions({
  inheritAttrs: false,
})

const normalizedOptions = computed(() => {
  return props.options.map((opt) => {
    const label = String((opt as Record<string, unknown>)[props.optionLabel] ?? '')
    const value = (opt as Record<string, unknown>)[props.optionValue] ?? ''
    return { label, value }
  })
})

const selectedLabel = computed(() => {
  const current = normalizedOptions.value.find((opt) => String(opt.value) === String(props.modelValue))
  return current ? current.label : undefined
})

function handleUpdate(val: unknown) {
  const match = normalizedOptions.value.find((opt) => String(opt.value) === String(val))
  const resolvedValue = match ? match.value : val
  emit('update:modelValue', resolvedValue)
}
</script>
