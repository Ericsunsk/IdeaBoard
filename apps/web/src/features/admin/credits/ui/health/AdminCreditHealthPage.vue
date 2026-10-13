<template>
  <div :class="adminDashboardRootClass">
    <div class="flex w-full flex-col overflow-hidden rounded-xl border border-border bg-card">
      <div v-if="showLoading" class="flex min-h-72 items-center justify-center">
        <AdminSpinner
          class="size-9 text-primary"
          :aria-label="CREDIT_HEALTH_MESSAGES.loadingAria"
        />
      </div>

      <div
        v-else-if="loadError"
        class="flex min-h-72 flex-col items-center justify-center px-6 py-12 text-center"
      >
        <XmarkCircle class="size-8 text-destructive" aria-hidden="true" />
        <h2 class="mt-4 text-admin-title font-bold text-foreground">
          {{ CREDIT_HEALTH_MESSAGES.unavailableTitle }}
        </h2>
        <p class="mt-2 text-admin-body text-muted-foreground">
          {{ CREDIT_HEALTH_MESSAGES.loadFailedHint }}
        </p>
        <ActionButton
          class="mt-5"
          variant="outline"
          :loading="resource.query.isFetching.value"
          @click="resource.query.refetch()"
        >
          {{ CREDIT_HEALTH_MESSAGES.reload }}
        </ActionButton>
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
                <h2 class="m-0 text-admin-title font-bold text-foreground">
                  {{ presentation.label }}
                </h2>
                <span
                  v-if="health.active_run"
                  class="rounded-full border border-border bg-muted px-2.5 py-0.5 text-xs font-semibold text-muted-foreground"
                >
                  {{ CREDIT_HEALTH_MESSAGES.activeRunBadge(health.active_run.id) }}
                </span>
              </div>
              <p class="mt-1.5 text-admin-body leading-6 text-muted-foreground">
                {{ presentation.summary }}
              </p>
            </div>
          </div>

          <div class="flex shrink-0 flex-wrap items-center gap-2">
            <ActionButton variant="outline" @click="historyDialogVisible = true">
              {{ CREDIT_HEALTH_MESSAGES.history }}
            </ActionButton>
            <ActionButton
              :disabled="Boolean(health.active_run) || resource.startPending.value"
              :loading="resource.startPending.value"
              @click="startReconciliation"
            >
              {{
                health.active_run ? CREDIT_HEALTH_MESSAGES.checking : CREDIT_HEALTH_MESSAGES.recheck
              }}
            </ActionButton>
          </div>
        </div>

        <AdminAlert v-if="resource.query.isError.value" class="mx-6 mb-5" severity="secondary">
          <div class="flex flex-wrap items-center justify-between gap-3">
            <span>{{ CREDIT_HEALTH_MESSAGES.staleNotice }}</span>
            <AdminButton
              variant="ghost"
              size="small"
              :class="adminTextButtonClass"
              :loading="resource.query.isFetching.value"
              @click="resource.query.refetch()"
            >
              {{ CREDIT_HEALTH_MESSAGES.retryRefresh }}
            </AdminButton>
          </div>
        </AdminAlert>

        <div
          class="grid border-t border-border bg-muted/30 sm:grid-cols-3 sm:divide-x sm:divide-border"
        >
          <div
            v-for="fact in facts"
            :key="fact.label"
            class="min-w-0 border-b border-border px-6 py-4.5 last:border-b-0 sm:border-b-0"
          >
            <div class="text-xs font-semibold tracking-wide text-muted-foreground">
              {{ fact.label }}
            </div>
            <div
              class="mt-1.5 truncate font-mono text-base font-bold text-foreground"
              :title="fact.value"
            >
              {{ fact.value }}
            </div>
            <div v-if="fact.detail" class="mt-0.5 text-xs text-muted-foreground">
              {{ fact.detail }}
            </div>
          </div>
        </div>

        <section
          :aria-label="CREDIT_HEALTH_MESSAGES.recentSectionAria"
          class="border-t border-border px-6 py-4"
        >
          <h3 class="text-sm font-bold text-foreground">
            {{ CREDIT_HEALTH_MESSAGES.recentSectionTitle }}
          </h3>
          <ul v-if="recentRuns.length > 0" class="mt-2 divide-y divide-border">
            <li
              v-for="{ kind, run } in recentRuns"
              :key="`${kind}-${run.id}`"
              class="flex min-w-0 flex-wrap items-center gap-x-3 gap-y-1 py-2.5"
            >
              <AdminStatusPill
                :label="getRunStatusLabel(run.status)"
                :variant="getRunStatusVariant(run.status)"
                size="small"
              />
              <span class="font-mono text-xs font-bold text-foreground"> Run #{{ run.id }} </span>
              <span class="text-xs text-muted-foreground">{{
                getReconciliationTriggerLabel(run)
              }}</span>
              <span class="min-w-0 flex-1 truncate text-xs text-muted-foreground">
                {{ formatReconciliationRunResult(run) }}
              </span>
              <span class="font-mono text-xs whitespace-nowrap text-muted-foreground">
                {{ formatRecentRunTime(run) }} · {{ formatReconciliationRunDuration(run) }}
              </span>
              <span
                v-if="kind === 'failed' && run.error_message"
                class="w-full truncate font-mono text-xs text-destructive"
                :title="formatReconciliationRunError(run.error_message)"
              >
                {{ formatReconciliationRunError(run.error_message) }}
              </span>
            </li>
          </ul>
          <p v-else class="mt-2 text-xs text-muted-foreground">
            {{ CREDIT_HEALTH_MESSAGES.emptyRuns }}
          </p>
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
import { AdminAlert } from '../../../shared'
import { AdminStatusPill } from '../../../shared'
import { AdminButton } from '../../../shared'
import ActionButton from '../../../../../core/components/ActionButton.vue'
import { AdminSpinner } from '../../../shared'
import { formatAdminDisplayError, useAdminToast } from '../../../shared'
import { adminDashboardRootClass, adminTextButtonClass } from '../../../shared'
import {
  formatAdminDateTimeWithSeconds,
  formatReconciliationRunError,
  getRunStatusLabel,
  getRunStatusVariant,
} from '../../core/shared/format'
import AdminCreditFindingQueue from './AdminCreditFindingQueue.vue'
import AdminCreditRunHistoryDialog from './AdminCreditRunHistoryDialog.vue'
import { useAdminCreditHealthResource } from '../../session/health/resource'
import { getCreditHealthPresentation } from '../../core/health/creditHealthPresentation'
import type { AdminCreditRun } from '../../session/health/runHistoryResource'
import {
  formatReconciliationRunDuration,
  formatReconciliationRunResult,
  getReconciliationTriggerLabel,
} from '../../core/health/reconciliationRunPresentation'
import { CREDIT_HEALTH_MESSAGES } from './messages'

const toast = useAdminToast()
const resource = useAdminCreditHealthResource()
const historyDialogVisible = ref(false)
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
      label: CREDIT_HEALTH_MESSAGES.openFindingsLabel,
      value: String(value.open_findings.total),
      detail: CREDIT_HEALTH_MESSAGES.openFindingsDetail(
        value.open_findings.warning,
        value.open_findings.error,
      ),
    },
    {
      label: CREDIT_HEALTH_MESSAGES.highRiskLabel,
      value: String(value.open_findings.high_risk),
      detail: CREDIT_HEALTH_MESSAGES.highRiskDetail(value.open_findings.critical),
    },
    {
      label: CREDIT_HEALTH_MESSAGES.restrictedWalletLabel,
      value: String(value.restricted_wallet_count),
      detail: CREDIT_HEALTH_MESSAGES.restrictedWalletDetail,
    },
  ]
})

type RecentRunKind = 'active' | 'success' | 'failed'

// 最近检查：进行中（如有）+ 最近成功 + 最近失败，一眼可及，无需打开检查记录。
const recentRuns = computed<Array<{ kind: RecentRunKind; run: AdminCreditRun }>>(() => {
  const value = health.value
  if (!value) return []
  const runs: Array<{ kind: RecentRunKind; run: AdminCreditRun }> = []
  if (value.active_run) runs.push({ kind: 'active', run: value.active_run })
  if (value.latest_successful_run) runs.push({ kind: 'success', run: value.latest_successful_run })
  if (value.latest_failed_run) runs.push({ kind: 'failed', run: value.latest_failed_run })
  return runs
})

function formatRecentRunTime(run: AdminCreditRun): string {
  return formatAdminDateTimeWithSeconds(run.completed_at ?? run.started_at ?? run.requested_at)
}

async function startReconciliation() {
  try {
    await resource.start()
    toast.success(CREDIT_HEALTH_MESSAGES.checkStarted)
  } catch (error) {
    toast.error(formatAdminDisplayError(error, CREDIT_HEALTH_MESSAGES.checkStartFailed))
  }
}
</script>
