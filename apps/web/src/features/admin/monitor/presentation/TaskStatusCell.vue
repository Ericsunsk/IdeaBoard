<template>
  <AdminStatusPill :label="pill.label" :variant="pill.variant" size="small" />
</template>

<script setup lang="ts">
import { computed } from 'vue'
import AdminStatusPill from '../../shared/components/AdminStatusPill.vue'
import { getTaskStatusDefinition } from '../domain/status'
import type { AdminTaskRow } from '../application/useAdminTaskList'

const props = defineProps<{
  row: AdminTaskRow
}>()

// 任务五态中文文案与 variant 均取 domain/status 单源。
const pill = computed(() => {
  const definition = getTaskStatusDefinition(props.row.status)
  return { label: definition.text, variant: definition.variant }
})
</script>
