<template>
  <div
    v-if="task.status === adminTaskStatuses.failed"
    class="flex items-start gap-2 rounded-panel border border-destructive/20 bg-destructive/5 p-3 text-xs leading-relaxed text-destructive"
    role="alert"
  >
    <WarningTriangle class="mt-0.5 size-3.5 shrink-0" />
    <div class="flex min-w-0 flex-1 flex-col gap-1">
      <div class="flex items-center justify-between">
        <span class="font-semibold">{{ MONITOR_MESSAGES.detailFailedTitle }}</span>
        <button
          type="button"
          class="h-5 cursor-pointer rounded-control border border-destructive/30 px-1.5 text-admin-caption font-medium text-destructive transition-colors duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] hover:bg-destructive/10"
          @click="$emit('retry')"
        >
          {{ MONITOR_MESSAGES.retry }}
        </button>
      </div>
      <span class="text-destructive/90">
        {{ getErrorReason(task) || MONITOR_MESSAGES.detailUnknownError }}
      </span>
    </div>
  </div>

  <div
    v-else-if="task.status === adminTaskStatuses.cancelled"
    class="flex items-start gap-2 rounded-panel border border-border bg-muted p-3 text-xs leading-relaxed text-muted-foreground"
  >
    <WarningTriangle class="mt-0.5 size-3.5 shrink-0 text-muted-foreground" />
    <div class="flex min-w-0 flex-1 flex-col">
      <span class="font-semibold">{{ MONITOR_MESSAGES.detailStoppedTitle }}</span>
      <span class="text-muted-foreground">
        {{ getErrorReason(task) || MONITOR_MESSAGES.detailStoppedFallback }}
      </span>
    </div>
  </div>

  <div
    v-else
    class="flex items-center justify-between rounded-panel border border-border bg-muted/50 px-3.5 py-2 text-xs"
  >
    <div class="flex items-center gap-1.5">
      <span class="text-muted-foreground">{{ MONITOR_MESSAGES.detailDurationLabel }}</span>
      <span class="font-mono font-medium text-foreground">{{ calcDuration(task) }}</span>
    </div>
    <div class="flex items-center gap-1.5">
      <span class="text-muted-foreground">{{ MONITOR_MESSAGES.detailCreatedLabel }}</span>
      <span class="font-mono font-medium text-foreground">{{
        formatDateTime(task.created_at)
      }}</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { WarningTriangle } from '@iconoir/vue'
import type { AdminMonitorDetailModel } from '../resource'
import { adminTaskStatuses } from '../core/status'
import { MONITOR_MESSAGES } from './pages/messages'

defineProps<{
  task: AdminMonitorDetailModel
  getErrorReason: (task: AdminMonitorDetailModel) => string
  formatDateTime: (value: string | null | undefined) => string
  calcDuration: (task: AdminMonitorDetailModel) => string
}>()
defineEmits<{ retry: [] }>()
</script>
