<template>
  <div
    :class="
      cn(
        'break-inside-avoid overflow-hidden rounded-xl border border-border bg-card text-foreground transition-colors',
        props.class,
      )
    "
  >
    <div
      class="flex min-w-0 items-center justify-between gap-3 border-b border-border bg-muted/30 px-5 py-3.5"
    >
      <span class="truncate text-admin-title-sm font-bold text-foreground">
        {{ title }}
      </span>
      <AdminBadge :value="status" class="shrink-0" :severity="statusSeverity" rounded />
    </div>

    <div class="flex flex-col gap-5 p-5">
      <slot />
      <div
        v-if="$slots.actions"
        class="mt-1 flex flex-wrap items-center justify-end gap-2 border-t border-border pt-4"
      >
        <slot name="actions" />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import AdminBadge from '../shared/components/AdminBadge.vue'
import { cn } from '@/lib/utils'

const props = withDefaults(
  defineProps<{
    class?: HTMLAttributes['class']
    status: string
    statusSeverity?: 'danger' | 'info' | 'secondary' | 'success' | 'warn'
    title: string
  }>(),
  { class: undefined, statusSeverity: 'secondary' },
)
</script>
