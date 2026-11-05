<template>
  <StatusBadge :label="pill.label" :tone="pill.tone" size="small" />
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { StatusBadge } from '../../shared'
import { getTaskStatusDefinition } from '../core/status'
import type { AdminTaskRow } from '../session/useAdminTaskList'

const props = defineProps<{
  row: AdminTaskRow
}>()

// 任务五态英文文案与 tone 均取 core/status 单源。
const pill = computed(() => {
  const definition = getTaskStatusDefinition(props.row.status)
  return { label: definition.text, tone: definition.tone }
})
</script>
