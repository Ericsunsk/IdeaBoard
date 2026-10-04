<template>
  <div :aria-label="BILLING_PRICING_MESSAGES.poolSummaryAria(props.displayName)">
    <span v-if="props.failed" class="truncate font-mono text-xs text-muted-foreground">
      {{ BILLING_PRICING_MESSAGES.poolLoadFailed }}
    </span>
    <span
      v-else-if="!props.pool || props.pool.entries.length === 0"
      class="truncate font-mono text-xs text-muted-foreground"
    >
      {{ BILLING_PRICING_MESSAGES.poolEmpty }}
    </span>
    <ul v-else class="flex min-w-0 flex-col gap-1">
      <li
        v-for="entry in props.pool.entries"
        :key="entry.modelId"
        class="flex min-w-0 items-center gap-1.5"
        :class="entry.enabled ? undefined : 'opacity-55'"
      >
        <span class="truncate font-mono text-xs text-foreground">{{ entry.modelId }}</span>
        <span class="flex shrink-0 items-center gap-0.5">
          <span
            v-for="resolution in entry.resolutions"
            :key="resolution"
            class="rounded bg-muted px-1 py-px font-mono text-admin-caption font-medium text-muted-foreground"
          >
            {{ resolution.toUpperCase() }}
          </span>
        </span>
      </li>
    </ul>
  </div>
</template>

<script setup lang="ts">
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
