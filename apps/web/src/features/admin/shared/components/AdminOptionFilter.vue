<template>
  <!--
    Filter-style dropdown. Builds on AdminSelect (reka `Select`) rather than a
    hand-rolled Popover: the previous implementation declared `role="listbox"`
    and `aria-haspopup="listbox"` on a plain Popover, which advertised a
    composite widget it did not implement — no arrow-key navigation, no
    typeahead, no Home/End. Reka's Select supplies the whole keyboard model.
    `active` carries the non-default filter state onto the shared trigger skin.
  -->
  <AdminSelect
    :model-value="selectValue"
    :options="normalizedOptions"
    :active="active"
    :aria-label="label"
    size="small"
    class="w-auto"
    @update:model-value="handleUpdate"
  />
</template>

<script setup lang="ts">
import { computed } from 'vue'
import AdminSelect from './AdminSelect.vue'

export type AdminOptionFilterOption = {
  label: string
  value: string
}

const props = withDefaults(
  defineProps<{
    modelValue: string
    options: readonly AdminOptionFilterOption[]
    allValue?: string
    label: string
  }>(),
  { allValue: '' },
)

const emit = defineEmits<{
  'update:modelValue': [value: string]
}>()

/**
 * reka `Select` reserves the empty string for "show the placeholder", so it
 * refuses a `<SelectItem value="">`. Filter lists legitimately carry an "all"
 * option whose value is `''` (see `statusFilterOptions` in
 * AdminBillingPricingPage), so the "all" value is mapped to a sentinel at this
 * boundary and translated back on the way out. Keeping the sentinel here means
 * callers keep passing their real domain values and never learn about it.
 */
const ALL_SENTINEL = '__all__'

/** True when the selection is a real filter rather than the "all" default. */
const active = computed(() => props.modelValue !== props.allValue)

const normalizedOptions = computed(() =>
  props.options.map((option) => ({
    label: option.label,
    value: option.value === props.allValue ? ALL_SENTINEL : option.value,
  })),
)

/** Selected value in reka's value space (sentinel-aware). */
const selectValue = computed(() => {
  const value = props.modelValue === props.allValue ? ALL_SENTINEL : props.modelValue
  return normalizedOptions.value.some((option) => option.value === value)
    ? value
    : normalizedOptions.value[0]?.value ?? ''
})

function handleUpdate(value: string | number) {
  emit('update:modelValue', value === ALL_SENTINEL ? props.allValue : String(value))
}
</script>
