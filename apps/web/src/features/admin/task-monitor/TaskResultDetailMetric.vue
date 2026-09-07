<template>
  <div
    class="flex min-w-0 flex-col gap-1 rounded-[14px] border border-admin-border bg-admin-surface-muted/70 px-3 py-2.5"
  >
    <span class="text-admin-caption font-bold text-admin-text-3">{{ label }}</span>
    <span
      class="min-w-0 truncate text-admin-body font-semibold text-admin-text-1"
      :title="title || value"
    >
      {{ value }}
    </span>
  </div>
</template>

<script setup lang="ts">
withDefaults(
  defineProps<{
    label: string
    value: string
    title?: string | undefined
  }>(),
  {
    title: undefined,
  },
)
</script>
