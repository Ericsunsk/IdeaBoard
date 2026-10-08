<template>
  <div :aria-label="BILLING_PRICING_MESSAGES.poolSummaryAria(props.displayName)">
    <AdminMono v-if="props.failed" class="truncate text-xs text-muted-foreground">
      {{ BILLING_PRICING_MESSAGES.poolLoadFailed }}
    </AdminMono>
    <AdminMono
      v-else-if="!props.pool || props.pool.entries.length === 0"
      class="truncate text-xs text-muted-foreground"
    >
      {{ BILLING_PRICING_MESSAGES.poolEmpty }}
    </AdminMono>
    <ul v-else class="flex min-w-0 flex-col gap-1">
      <li
        v-for="entry in props.pool.entries"
        :key="entry.modelId"
        class="flex min-w-0 items-center gap-1.5"
        :class="entry.enabled ? undefined : 'opacity-55'"
      >
        <AdminMono class="truncate text-xs text-foreground">{{ entry.modelId }}</AdminMono>
        <span class="flex shrink-0 items-center gap-0.5">
          <AdminMono
            v-for="resolution in entry.resolutions"
            :key="resolution"
            class="rounded bg-muted px-1 py-px text-admin-caption font-medium text-muted-foreground"
          >
            {{ resolution.toUpperCase() }}
          </AdminMono>
        </span>
      </li>
    </ul>
  </div>
</template>

<script setup lang="ts">
import AdminMono from '../../shared/components/AdminMono.vue'
import type { AdminModelPool } from './billableCatalogTypes'
import { BILLING_PRICING_MESSAGES } from './messages'

// Compact per-Service Model Pool summary: one row per Model with its
// supported resolution badges (`Model A [1K][2K][4K]`). Disabled entries
// stay visible but dimmed; an empty pool renders 未配置.
const props = withDefaults(
  defineProps<{
    displayName: string
    pool?: AdminModelPool | null | undefined
    failed?: boolean | undefined
  }>(),
  {
    pool: null,
    failed: false,
  },
)
</script>
