<template>
  <span
    :class="[
      'inline-flex items-center gap-1 rounded px-1.5 py-0.5 font-mono text-[11px] font-medium leading-none',
      metric.activePillClass,
    ]"
  >
    <span :class="['size-1.5 rounded-full', metric.dotClass]" />
    <span>{{ label }}</span>
  </span>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import {
  taskMetricDefinitions,
  type TaskMetricKey,
} from '../../../../shared/domain/taskMetricTokens'
import type { AdminBillableItemStatus } from './billableCatalogTypes'

const props = defineProps<{
  status: AdminBillableItemStatus
}>()

// 定价状态借任务监控 metric token（圆点+胶囊同系），文案保留定价四态：
// 未定价→pending 蓝，生效中→completed 绿，已停用/已归档→cancelled 灰。
const metricKeyForStatus: Record<AdminBillableItemStatus, TaskMetricKey> = {
  unpriced: 'pending',
  active: 'completed',
  disabled: 'cancelled',
  archived: 'cancelled',
}

const statusLabel: Record<AdminBillableItemStatus, string> = {
  unpriced: '未定价',
  active: '生效中',
  disabled: '已停用',
  archived: '已归档',
}

const metric = computed(
  () =>
    taskMetricDefinitions.find((item) => item.key === metricKeyForStatus[props.status]) ??
    taskMetricDefinitions[0]!,
)
const label = computed(() => statusLabel[props.status])
</script>
