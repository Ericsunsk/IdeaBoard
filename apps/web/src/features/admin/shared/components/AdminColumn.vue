<template><slot /></template>

<script setup lang="ts" generic="TRow extends object">
defineProps<{
  field?: string | undefined
  header?: string | undefined
  style?: string | Record<string, string | number> | undefined
  selectionMode?: 'multiple' | undefined
  expander?: boolean | undefined
}>()

// Slot props mirror the PrimeVue Column body contract so migrated templates
// keep working unchanged. Rows vary per table and pass through untouched,
// hence the intentionally loose data type (same looseness as its predecessor).
defineSlots<{
  // eslint-disable-next-line @typescript-eslint/no-explicit-any -- table rows are heterogeneous by design; narrowing here would force casts at all 50+ call sites
  body?: (props: { data: any; field?: string | undefined; index?: number | undefined }) => any
  // eslint-disable-next-line @typescript-eslint/no-explicit-any -- slot render output can be any VNode/renderable
  header?: () => any
  // eslint-disable-next-line @typescript-eslint/no-explicit-any -- passthrough outlet for direct mounting; the table reads (never renders) column vnodes
  default?: () => any
}>()
</script>
