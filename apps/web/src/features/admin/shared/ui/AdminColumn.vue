<template><slot /></template>

<script setup lang="ts" generic="TRow extends object = any">
defineProps<{
  field?: string | undefined
  header?: string | undefined
  style?: string | Record<string, string | number> | undefined
  selectionMode?: 'multiple' | undefined
  expander?: boolean | undefined
}>()

// Slot props mirror the column body contract so templates
// keep working unchanged. Rows vary per table and pass through untouched.
defineSlots<{
  // eslint-disable-next-line @typescript-eslint/no-explicit-any -- slot render output can be any VNode/renderable
  body?: (props: { data: TRow; field?: string | undefined; index?: number | undefined }) => any
  // eslint-disable-next-line @typescript-eslint/no-explicit-any -- slot render output can be any VNode/renderable
  header?: () => any
  // eslint-disable-next-line @typescript-eslint/no-explicit-any -- passthrough outlet for direct mounting; the table reads (never renders) column vnodes
  default?: () => any
}>()
</script>
