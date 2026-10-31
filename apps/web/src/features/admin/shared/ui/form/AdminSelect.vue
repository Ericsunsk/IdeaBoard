<template>
  <!--
    The trigger's accessible name is supplied per call site: `ariaLabel` for
    filter-style selects, or a visible <label for> bound to `id` for form
    selects. `vuejs-accessibility/form-control-has-label` cannot follow either
    across a component boundary, so the association is pinned by
    adminSelectSkin / AdminOptionFilter specs and the a11y smoke lane instead.
  -->
  <!-- eslint-disable-next-line vuejs-accessibility/form-control-has-label -->
  <Select
    :model-value="modelValue != null ? String(modelValue) : ''"
    :disabled="disabled"
    @update:model-value="handleUpdate"
  >
    <SelectTrigger
      :id="id"
      v-bind="$attrs"
      :aria-label="props.ariaLabel ?? ($attrs['aria-label'] as string | undefined)"
      :class="
        cn(
          adminSelectTriggerClass,
          size === 'small' && adminSelectTriggerCompactClass,
          active && adminSelectTriggerActiveClass,
          props.class,
        )
      "
    >
      <SelectValue :placeholder="placeholder">
        {{ selectedLabel || placeholder }}
      </SelectValue>
      <SelectIcon>
        <NavArrowDown :class="adminSelectTriggerIconClass" />
      </SelectIcon>
    </SelectTrigger>

    <SelectContent
      :class="adminSelectContentClass"
      position="popper"
      :side-offset="ADMIN_SELECT_SIDE_OFFSET"
    >
      <SelectViewport :class="adminSelectViewportClass">
        <slot>
          <SelectItem
            v-for="opt in normalizedOptions"
            :key="String(opt.value)"
            :value="String(opt.value)"
            :class="cn(adminSelectItemClass, size === 'small' && adminSelectItemCompactClass)"
          >
            <span :class="adminSelectItemIndicatorClass">
              <SelectItemIndicator>
                <Check :class="adminSelectItemCheckClass" />
              </SelectItemIndicator>
            </span>
            <SelectItemText>{{ opt.label }}</SelectItemText>
          </SelectItem>
        </slot>
      </SelectViewport>
    </SelectContent>
  </Select>
</template>

<script setup lang="ts">
import { computed, type HTMLAttributes } from 'vue'
import {
  Select,
  SelectContent,
  SelectIcon,
  SelectItem,
  SelectItemIndicator,
  SelectItemText,
  SelectTrigger,
  SelectValue,
  SelectViewport,
} from '@/components/ui/select'
import { Check, NavArrowDown } from '@iconoir/vue'
import { cn } from '@/lib/utils'
import {
  ADMIN_SELECT_SIDE_OFFSET,
  adminSelectContentClass,
  adminSelectItemCheckClass,
  adminSelectItemClass,
  adminSelectItemCompactClass,
  adminSelectItemIndicatorClass,
  adminSelectTriggerActiveClass,
  adminSelectTriggerClass,
  adminSelectTriggerCompactClass,
  adminSelectTriggerIconClass,
  adminSelectViewportClass,
} from '../styling/adminSelectSkin'

/**
 * Admin skin adapter for the neutral shared Select primitive.
 *
 * Structure and the keyboard model (arrow keys, typeahead, Home/End, Escape)
 * come from `@/components/ui/select`; this file only supplies Admin tokens, per
 * the same split `Popover` and `DropdownMenu` callers rely on. Route pages never
 * import `components/ui/select` directly.
 */
export type SelectOptionItem = {
  label: string
  value: string | number
}

const props = withDefaults(
  defineProps<{
    id?: string | undefined
    /**
     * Accessible name for the trigger. Select's trigger is a real form control,
     * so it needs a programmatic label; callers that render a visible <label>
     * pass its `for` via `id` instead and can leave this unset.
     */
    ariaLabel?: string | undefined
    modelValue?: string | number | null | undefined
    options?: readonly (SelectOptionItem | Record<string, unknown>)[]
    optionLabel?: string
    optionValue?: string
    placeholder?: string
    class?: HTMLAttributes['class']
    disabled?: boolean
    size?: 'small' | 'default'
    /** Marks the trigger as carrying a non-default filter value. */
    active?: boolean
  }>(),
  {
    id: undefined,
    ariaLabel: undefined,
    modelValue: '',
    options: () => [],
    optionLabel: 'label',
    optionValue: 'value',
    placeholder: '请选择...',
    class: undefined,
    disabled: false,
    size: 'default',
    active: false,
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
  const current = normalizedOptions.value.find(
    (opt) => String(opt.value) === String(props.modelValue),
  )
  return current ? current.label : undefined
})

function handleUpdate(val: unknown) {
  const match = normalizedOptions.value.find((opt) => String(opt.value) === String(val))
  const resolvedValue = match ? match.value : val
  emit('update:modelValue', resolvedValue)
}
</script>
