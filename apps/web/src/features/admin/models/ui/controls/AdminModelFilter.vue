<template>
  <!--
    One model-dimension filter (mode / provider / hosting). Builds on
    AdminSelect (reka `Select`) rather than a hand-rolled Popover: the previous
    implementation put `role="listbox"` on a plain `AdminPopoverContent` over
    bare `<button role="option">` children, which promises the APG composite
    model — one Tab stop, arrow keys between options, Home/End, typeahead — that
    it never implemented. reka's Select supplies the whole keyboard model, and
    AdminSelect supplies the one Admin skin every other dropdown uses.

    `placeholder` is only a fallback: while the "all" sentinel is selected the
    trigger states the current value (`全部类型`), exactly like every other Admin
    filter, and it falls back to `label` when the workspace filter points at a
    value the catalog no longer lists. `active` paints the accent border once a
    real filter is applied.
  -->
  <AdminSelect
    :model-value="selectValue"
    :options="normalizedOptions"
    :aria-label="ariaLabel ?? `筛选${label}`"
    :placeholder="label"
    :active="modelValue !== ''"
    class="w-auto"
    @update:model-value="handleUpdate"
  />
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { AdminSelect } from '../../../shared'

const props = defineProps<{
  label: string
  allLabel: string
  options: readonly string[]
  ariaLabel?: string | undefined
}>()

const modelValue = defineModel<string>({ required: true })

/**
 * reka `Select` reserves the empty string for "show the placeholder" and
 * rejects a `<SelectItem value="">`. This page models "no filter" as `''` (see
 * `useAdminModelCatalogWorkspace`), and the filter must stay reachable once a
 * value is picked, so `''` is mapped to a sentinel at this boundary and
 * translated back on the way out. `AdminOptionFilter` applies the same mapping
 * for `{ label, value }` filter lists; it cannot be reused here because it also
 * owns the trigger's `size` and idle text, which this page pins to `h-9` and to
 * the dimension label.
 */
const ALL_SENTINEL = '__all__'

/** `{ allLabel }` first, then one item per option, in the page's order. */
const normalizedOptions = computed(() => [
  { label: props.allLabel, value: ALL_SENTINEL },
  ...props.options.map((option) => ({ label: option, value: option })),
])

/** Current value in reka's value space (empty string → sentinel). */
const selectValue = computed(() => (modelValue.value === '' ? ALL_SENTINEL : modelValue.value))

function handleUpdate(value: string | number) {
  modelValue.value = value === ALL_SENTINEL ? '' : String(value)
}
</script>
