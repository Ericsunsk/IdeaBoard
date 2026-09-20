<template>
  <div :class="adminDashboardRootClass">
    <div class="flex w-full flex-col overflow-hidden rounded-xl border border-admin-border-soft bg-white">
      <div
        v-if="showLoading"
        class="flex min-h-72 items-center justify-center"
      >
        <AdminSpinner class="size-9 text-admin-accent" aria-label="加载账本健康状态" />
      </div>

      <div
        v-else-if="loadError"
        class="flex min-h-72 flex-col items-center justify-center px-6 py-12 text-center"
      >
        <XmarkCircle class="size-8 text-admin-danger" aria-hidden="true" />
        <h2 class="mt-4 text-admin-title font-bold text-admin-title-color">账本健康状态不可用</h2>
        <p class="mt-2 text-admin-body text-admin-subtitle-color">数据加载失败，请重试。</p>
        <AdminButton
          class="mt-5"
          variant="outline"
          :class="adminActionButtonClass"
          :loading="resource.query.isFetching.value"
          @click="resource.query.refetch()"
        >
          重新加载
        </AdminButton>
      </div>

      <div v-else-if="health && presentation" class="min-w-0">
        <div class="flex flex-col gap-6 px-6 py-6 lg:flex-row lg:items-start lg:justify-between">
          <div class="flex min-w-0 items-start gap-4">
            <span class="mt-0.5 shrink-0" :class="presentation.iconClass" aria-hidden="true">
              <CheckCircle v-if="presentation.iconKey === 'healthy'" class="size-7" />
              <Clock v-else-if="presentation.iconKey === 'time'" class="size-7" />
              <XmarkCircle v-else-if="presentation.iconKey === 'failed'" class="size-7" />
              <WarningTriangle v-else class="size-7" />
            </span>
            <div class="min-w-0">
              <div class="flex flex-wrap items-center gap-2.5">
                <h2 class="m-0 text-admin-title font-bold text-admin-title-color">
                  {{ presentation.label }}
                </h2>
                <span
                  v-if="health.active_run"
                  class="rounded-full border border-admin-border-soft bg-admin-bg px-2.5 py-0.5 text-xs font-semibold text-admin-text-2"
                >
                  检查进行中 · Run #{{ health.active_run.id }}
                </span>
              </div>
              <p class="mt-1.5 text-admin-body leading-6 text-admin-subtitle-color">
                {{ presentation.summary }}
              </p>
            </div>
          </div>

          <div class="flex shrink-0 flex-wrap items-center gap-2">
            <AdminButton
              variant="outline"
              :class="adminActionButtonClass"
              @click="historyDialogVisible = true"
            >
              检查记录
            </AdminButton>
            <AdminButton
              :class="adminActionButtonClass"
              :disabled="Boolean(health.active_run) || resource.startPending.value"
              :loading="resource.startPending.value"
              @click="startReconciliation"
            >
              {{ health.active_run ? '检查中' : '重新检查' }}
            </AdminButton>
          </div>
        </div>

        <AdminAlert
          v-if="resource.query.isError.value"
          class="mx-6 mb-5"
          severity="secondary"
        >
          <div class="flex flex-wrap items-center justify-between gap-3">
            <span>状态刷新失败，当前显示上次加载的数据。</span>
            <AdminButton
              variant="ghost"
              size="small"
              :class="adminTextButtonClass"
              :loading="resource.query.isFetching.value"
              @click="resource.query.refetch()"
            >
              重试刷新
            </AdminButton>
          </div>
        </AdminAlert>

        <div
          class="grid border-t border-admin-border-soft bg-admin-bg/30 sm:grid-cols-3 sm:divide-x sm:divide-admin-border-soft"
        >
          <div
            v-for="fact in facts"
            :key="fact.label"
            class="min-w-0 border-b border-admin-border-soft px-6 py-4.5 last:border-b-0 sm:border-b-0"
          >
            <div class="text-xs font-semibold tracking-wide text-admin-text-3">
              {{ fact.label }}
            </div>
            <div
              class="mt-1.5 truncate font-mono text-base font-bold text-admin-text-1"
              :title="fact.value"
            >
              {{ fact.value }}
            </div>
            <div v-if="fact.detail" class="mt-0.5 text-xs text-admin-text-3">
              {{ fact.detail }}
            </div>
          </div>
        </div>

        <section
          aria-label="最近检查"
          class="border-t border-admin-border-soft px-6 py-4"
        >
          <h3 class="text-sm font-bold text-admin-title-color">最近检查</h3>
          <ul v-if="recentRuns.length > 0" class="mt-2 divide-y divide-admin-border-soft">
            <li
              v-for="{ kind, run } in recentRuns"
              :key="`${kind}-${run.id}`"
              class="flex min-w-0 flex-wrap items-center gap-x-3 gap-y-1 py-2.5"
            >
              <AdminBadge
                :value="getRunStatusLabel(run.status)"
                :severity="getRunStatusSeverity(run.status)"
                rounded
              />
              <span class="font-mono text-xs font-bold text-admin-text-1">
                Run #{{ run.id }}
              </span>
              <span class="text-xs text-admin-text-3">{{ getReconciliationTriggerLabel(run) }}</span>
              <span class="min-w-0 flex-1 truncate text-xs text-admin-text-2">
                {{ formatReconciliationRunResult(run) }}
              </span>
              <span class="font-mono text-xs whitespace-nowrap text-admin-text-3">
                {{ formatRecentRunTime(run) }} · {{ formatReconciliationRunDuration(run) }}
              </span>
              <span
                v-if="kind === 'failed' && run.error_message"
                class="w-full truncate font-mono text-xs text-admin-danger"
                :title="formatReconciliationRunError(run.error_message)"
              >
                {{ formatReconciliationRunError(run.error_message) }}
              </span>
            </li>
          </ul>
          <p v-else class="mt-2 text-xs text-admin-text-3">暂无检查记录</p>
        </section>
      </div>
    </div>

    <AdminCreditFindingQueue />
  </div>

  <AdminCreditRunHistoryDialog v-model:visible="historyDialogVisible" />
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { CheckCircle, Clock, WarningTriangle, XmarkCircle } from '@iconoir/vue'
import type { CreditReconciliationRun } from '../../../../shared/api/generated-query/model'
import AdminAlert from '../../shared/components/AdminAlert.vue'
import AdminBadge from '../../shared/components/AdminBadge.vue'
import AdminButton from '../../shared/components/AdminButton.vue'
import AdminSpinner from '../../shared/components/AdminSpinner.vue'
import { formatAdminDisplayError, useAdminToast } from '../../shared/lib/toast'
import {
  adminActionButtonClass,
  adminDashboardRootClass,
  adminTextButtonClass,
} from '../../shared/lib/uiClasses'
import {
  formatAdminDateTimeWithSeconds,
  formatReconciliationRunError,
  getRunStatusLabel,
  getRunStatusSeverity,
} from '../shared/format'
import AdminCreditFindingQueue from './AdminCreditFindingQueue.vue'
import AdminCreditRunHistoryDialog from './AdminCreditRunHistoryDialog.vue'
import { useAdminCreditHealthResource } from './resource'
import { getCreditHealthPresentation } from './creditHealthPresentation'
import {
  formatReconciliationRunDuration,
  formatReconciliationRunResult,
  getReconciliationTriggerLabel,
} from './reconciliationRunPresentation'

const toast = useAdminToast()
const resource = useAdminCreditHealthResource()
const historyDialogVisible = ref(false)
const manualReconciliationReason = '管理员手动触发账本检查'
const health = resource.health
const showLoading = computed(() => resource.query.isLoading.value && !health.value)
const loadError = computed(() => resource.query.isError.value && !health.value)
const presentation = computed(() =>
  health.value ? getCreditHealthPresentation(health.value.status) : null,
)
const facts = computed(() => {
  const value = health.value
  if (!value) return []
  return [
    {
      label: '待处理异常',
      value: String(value.open_findings.total),
      detail: `${value.open_findings.warning} warning · ${value.open_findings.error} error`,
    },
    {
      label: '高风险异常',
      value: String(value.open_findings.high_risk),
      detail: `${value.open_findings.critical} critical`,
    },
    {
      label: '受限 Wallet',
      value: String(value.restricted_wallet_count),
      detail: 'Ledger Integrity',
    },
  ]
})

type RecentRunKind = 'active' | 'success' | 'failed'

// 最近检查：进行中（如有）+ 最近成功 + 最近失败，一眼可及，无需打开检查记录。
const recentRuns = computed<Array<{ kind: RecentRunKind; run: CreditReconciliationRun }>>(() => {
  const value = health.value
  if (!value) return []
  const runs: Array<{ kind: RecentRunKind; run: CreditReconciliationRun }> = []
  if (value.active_run) runs.push({ kind: 'active', run: value.active_run })
  if (value.latest_successful_run) runs.push({ kind: 'success', run: value.latest_successful_run })
  if (value.latest_failed_run) runs.push({ kind: 'failed', run: value.latest_failed_run })
  return runs
})

function formatRecentRunTime(run: CreditReconciliationRun): string {
  return formatAdminDateTimeWithSeconds(run.completed_at ?? run.started_at ?? run.requested_at)
}

async function startReconciliation() {
  try {
    await resource.start({ reason: manualReconciliationReason })
    toast.success('账本检查已启动')
  } catch (error) {
    toast.error(formatAdminDisplayError(error, '启动账本检查失败，请重试'))
  }
}
</script>
