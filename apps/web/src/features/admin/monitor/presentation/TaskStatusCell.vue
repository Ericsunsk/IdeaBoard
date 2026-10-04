<template>
  <span
    v-if="view.metric"
    :class="[
      'inline-flex items-center gap-1 rounded px-1.5 py-0.5 font-mono text-admin-caption font-medium leading-none',
      view.metric.activePillClass,
    ]"
  >
    <span :class="['size-1.5 rounded-full', view.metric.dotClass]" />
    <span>{{ view.metric.label }}</span>
  </span>
  <span v-else class="text-xs text-muted-foreground font-mono">
    {{ row.status }}
  </span>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { getTaskRowView } from './columns'
import type { AdminTaskRow } from '../application/useAdminTaskList'

const props = defineProps<{
  row: AdminTaskRow
}>()

const view = computed(() => getTaskRowView(props.row))
</script>
