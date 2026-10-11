<template>
  <!-- 防抖固定槽收口到 SwitchRow：四态最长 8 字符（Disabled/Unpriced/Archived）
    按 84px 锁宽，短文案 Active(6) 居中不收缩；pulse 默认关闭，
    success↔neutral 翻转不挂载/卸载 ping。纯 status 展示，不渲染 switch。 -->
  <SwitchRow :pill="label" :pill-variant="variant" :pulse="false" status-min-width="84px" />
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { SwitchRow } from '../../../shared'
import type { AdminStatusPillVariant } from '../../../shared'
import type { AdminBillableItemStatus } from '../../core/pricing/billableCatalogTypes'

const props = defineProps<{
  status: AdminBillableItemStatus
}>()

// 定价状态载体统一 AdminStatusPill（方案 A）：文案保留定价四态，
// variant 沿用原 taskMetricTokens 语义：生效中 success、未定价 info、
// 已停用/已归档 neutral（中性，不蹭健康色）。
const statusLabel: Record<AdminBillableItemStatus, string> = {
  unpriced: 'Unpriced',
  active: 'Active',
  disabled: 'Disabled',
  archived: 'Archived',
}

const statusVariant: Record<AdminBillableItemStatus, AdminStatusPillVariant> = {
  unpriced: 'info',
  active: 'success',
  disabled: 'neutral',
  archived: 'neutral',
}

const label = computed(() => statusLabel[props.status])
const variant = computed<AdminStatusPillVariant>(() => statusVariant[props.status])
</script>
