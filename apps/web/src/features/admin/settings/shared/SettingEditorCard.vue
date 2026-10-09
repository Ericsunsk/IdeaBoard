<template>
  <div
    :class="
      cn(
        plain
          ? 'flex flex-col gap-3.5'
          : 'break-inside-avoid overflow-hidden rounded-xl border border-border bg-card text-foreground transition-colors',
        props.class,
      )
    "
  >
    <div
      :class="
        cn(
          'flex min-w-0 items-center justify-between gap-3',
          plain ? 'pb-1' : 'border-b border-border bg-muted/30 px-5 py-3.5',
        )
      "
    >
      <div class="flex min-w-0 flex-col gap-0.5">
        <div class="flex items-center gap-2">
          <component
            :is="icon"
            v-if="icon"
            class="size-4 shrink-0 text-muted-foreground"
            aria-hidden="true"
          />
          <span class="truncate text-admin-title-sm font-bold text-foreground">
            {{ title }}
          </span>
        </div>
        <span v-if="description" class="text-admin-caption text-muted-foreground">
          {{ description }}
        </span>
      </div>
      <div v-if="$slots['header-actions'] || status" class="flex shrink-0 items-center gap-2">
        <slot name="header-actions" />
        <AdminBadge
          v-if="status"
          :value="status"
          class="shrink-0"
          :severity="statusSeverity"
          rounded
        />
      </div>
    </div>

    <div
      :class="
        cn(
          plain
            ? 'flex flex-col gap-4 rounded-xl border border-border bg-card p-5'
            : 'flex flex-col gap-3.5 p-5',
        )
      "
    >
      <slot />
      <div
        v-if="$slots.actions"
        :class="
          cn(
            'flex flex-wrap items-center justify-end gap-2',
            plain ? 'pt-1' : 'mt-1 border-t border-border pt-4',
          )
        "
      >
        <slot name="actions" />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { Component, HTMLAttributes } from 'vue'
import AdminBadge from '../../shared/components/AdminBadge.vue'
import { cn } from '@/lib/utils'

const props = withDefaults(
  defineProps<{
    class?: HTMLAttributes['class'] | undefined
    description?: string | undefined
    icon?: Component | undefined
    plain?: boolean | undefined
    status?: string | undefined
    statusSeverity?: 'danger' | 'info' | 'secondary' | 'success' | 'warn' | undefined
    title: string
  }>(),
  {
    plain: false,
    statusSeverity: 'secondary',
    class: undefined,
    description: undefined,
    icon: undefined,
    status: undefined,
  },
)
</script>
