<template>
  <AdminDialogShell
    :visible="visible"
    :header="CREDIT_HEALTH_MESSAGES.history"
    :description="CREDIT_HEALTH_MESSAGES.runHistoryDescription"
    size="wide"
    content-layout="scrollable"
    @update:visible="$emit('update:visible', $event)"
  >
    <AdminTablePanel>
      <template #toolbar>
        <AdminResourceToolbar>
          <template #start>
            <div class="min-w-0">
              <div class="text-admin-title-sm font-admin-display text-foreground">
                {{ CREDIT_HEALTH_MESSAGES.runHistoryTitle }}
              </div>
            </div>
          </template>
          <template #actions="{ iconButtonClass }">
            <AdminButton
              variant="ghost"
              :class="iconButtonClass"
              :aria-label="CREDIT_HEALTH_MESSAGES.runHistoryRefreshAria"
              :is-loading="runsLoading"
              @click="refreshRuns"
            >
              <template #icon><Refresh class="size-4" /></template>
            </AdminButton>
          </template>
          <template #pager>
            <AdminPagerControls
              :current-page="currentPage"
              :total-pages="knownPageCount"
              :prev-disabled="!canRetreat"
              :next-disabled="!canAdvance"
              @prev="previousPage"
              @next="nextPage"
            />
          </template>
        </AdminResourceToolbar>
      </template>

      <AdminAlert v-if="backgroundError" class="m-4" severity="secondary">
        <div class="flex flex-wrap items-center justify-between gap-3">
          <span>{{ CREDIT_HEALTH_MESSAGES.runHistoryStaleNotice }}</span>
          <AdminButton
            :label="CREDIT_HEALTH_MESSAGES.retryRefresh"
            severity="secondary"
            size="sm"
            variant="ghost"
            :is-loading="runsLoading"
            @click="refreshRuns"
          />
        </div>
      </AdminAlert>

      <AdminTable
        v-if="!initialError"
        v-model:expanded-rows="expandedRows"
        :value="runs"
        data-key="id"
        :loading="runsLoading"
        scrollable
        :frame-class="adminTableFrameClass"
        :table-style="{ minWidth: '820px' }"
        :class="adminTableClass"
        :table-aria-label="CREDIT_HEALTH_MESSAGES.runHistoryTableAria"
        :scroll-aria-label="CREDIT_HEALTH_MESSAGES.runHistoryScrollAria"
        @row-expand="expandRun"
        @row-collapse="collapseRun"
      >
        <template #empty>
          <AdminEmptyState
            :title="CREDIT_HEALTH_MESSAGES.emptyRuns"
            :description="CREDIT_HEALTH_MESSAGES.runHistoryEmptyHint"
          />
        </template>

        <AdminColumn expander :style="{ width: ADMIN_TABLE_EXPANDER_COLUMN_WIDTH }" />
        <AdminColumn header="Trigger" style="width: 120px">
          <template #body="{ data }">
            {{ getReconciliationTriggerLabel(data) }}
          </template>
        </AdminColumn>
        <AdminColumn :header="CREDIT_HEALTH_MESSAGES.runHistoryStatusColumn" style="width: 110px">
          <template #body="{ data }">
            <StatusBadge
              :label="getRunStatusLabel(data.status)"
              :tone="getRunStatusTone(data.status)"
              size="small"
            />
          </template>
        </AdminColumn>
        <AdminColumn
          :header="CREDIT_HEALTH_MESSAGES.runHistoryResultColumn"
          style="min-width: 240px"
        >
          <template #body="{ data }">{{ formatReconciliationRunResult(data) }}</template>
        </AdminColumn>
        <AdminColumn :header="CREDIT_HEALTH_MESSAGES.runHistoryTimeColumn" style="width: 190px">
          <template #body="{ data }">
            <span class="whitespace-nowrap">{{
              formatAdminDateTimeWithSeconds(data.requested_at)
            }}</span>
          </template>
        </AdminColumn>
        <AdminColumn :header="CREDIT_HEALTH_MESSAGES.runHistoryDurationColumn" style="width: 120px">
          <template #body="{ data }">{{ formatReconciliationRunDuration(data) }}</template>
        </AdminColumn>

        <template #expansion="{ data }">
          <div class="border-y border-border bg-muted px-5 py-4">
            <div
              v-if="runDetailLoading && selectedRunId === data.id"
              class="flex min-h-24 items-center justify-center"
              :aria-label="CREDIT_HEALTH_MESSAGES.runHistoryDetailLoadingAria"
            >
              <AdminSpinner
                class="h-7 w-7"
                :aria-label="CREDIT_HEALTH_MESSAGES.runHistoryDetailLoadingAria"
              />
            </div>
            <dl
              v-else-if="selectedRunId === data.id && selectedRun"
              class="grid gap-x-6 gap-y-4 sm:grid-cols-2 xl:grid-cols-4"
            >
              <div>
                <dt class="text-admin-caption font-bold text-muted-foreground">
                  {{ CREDIT_HEALTH_MESSAGES.runHistoryTechId }}
                </dt>
                <dd class="mt-1 mb-0 font-mono text-admin-body text-foreground">
                  Run #{{ selectedRun.id }}
                </dd>
              </div>
              <div>
                <dt class="text-admin-caption font-bold text-muted-foreground">
                  {{ CREDIT_HEALTH_MESSAGES.runHistoryAttempts }}
                </dt>
                <dd class="mt-1 mb-0 text-admin-body text-foreground">
                  {{ selectedRun.attempt_count }}
                </dd>
              </div>
              <div v-if="selectedRun.error_message" class="sm:col-span-2 xl:col-span-4">
                <dt class="text-admin-caption font-bold text-muted-foreground">
                  {{ CREDIT_HEALTH_MESSAGES.runHistoryError }}
                </dt>
                <dd class="mt-1 mb-0 break-words text-admin-body text-destructive">
                  {{ formatReconciliationRunError(selectedRun.error_message) }}
                </dd>
              </div>
            </dl>
          </div>
        </template>
      </AdminTable>
    </AdminTablePanel>

    <template #footer>
      <ActionButton variant="outline" @click="$emit('update:visible', false)">
        {{ CREDIT_HEALTH_MESSAGES.close }}
      </ActionButton>
    </template>
  </AdminDialogShell>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useQueryClient } from '@tanstack/vue-query'
import { useIntervalFn } from '@vueuse/core'
import { Refresh } from '@iconoir/vue'
import { AdminAlert } from '../../../shared'
import { StatusBadge } from '../../../shared'
import { AdminButton } from '../../../shared'
import ActionButton from '@/core/components/ActionButton.vue'
import { AdminColumn } from '../../../shared'
import { AdminSpinner } from '../../../shared'
import { AdminTable } from '../../../shared'
import type { AdminTableRowCollapseEvent, AdminTableRowExpandEvent } from '../../../shared'
import { AdminDialogShell } from '../../../shared'
import { AdminEmptyState } from '../../../shared'
import { AdminPagerControls } from '../../../shared'
import { AdminResourceToolbar } from '../../../shared'
import { AdminTablePanel } from '../../../shared'
import { useAdminCursorCollection } from '../../../shared'
import {
  formatAdminDateTimeWithSeconds,
  formatReconciliationRunError,
  getRunStatusLabel,
  getRunStatusTone,
} from '../../core/shared/format'
import { adminTableClass, adminTableFrameClass } from '../../../shared'
import { ADMIN_TABLE_EXPANDER_COLUMN_WIDTH } from '../../../shared'
import {
  createAdminCreditRunHistoryResource,
  type AdminCreditRun,
} from '../../session/health/runHistoryResource'
import {
  formatReconciliationRunDuration,
  formatReconciliationRunResult,
  getReconciliationTriggerLabel,
} from '../../core/health/reconciliationRunPresentation'
import { useCreditRunDetail } from '../../session/health/useCreditRunDetail'
import { useCreditHealthFeedback } from '../../session/health/useCreditHealthFeedback'
import { useFailureToast } from '@/core/composables/useFailureToast'
import { CREDIT_HEALTH_MESSAGES } from './messages'

const props = defineProps<{ visible: boolean }>()
defineEmits<{ 'update:visible': [visible: boolean] }>()

const feedback = useCreditHealthFeedback()
const expandedRows = ref<Record<string, boolean>>({})
const resource = createAdminCreditRunHistoryResource()
const queryClient = useQueryClient()
// 游标集合生命周期收敛于 useAdminCursorCollection，query 挂载前保持 disabled，
// 关闭即清缓存以便重开必转 fresh。
const RUN_HISTORY_QUERY_ROOT = ['admin', 'credits', 'run-history'] as const
const RUN_HISTORY_CURSOR_INDEX = RUN_HISTORY_QUERY_ROOT.length
const {
  data: runPage,
  loading: runsLoading,
  initialError,
  backgroundError,
  refresh: refreshRunHistory,
  query: runHistoryQuery,
  pagination,
} = useAdminCursorCollection({
  buildKey: (beforeId) => [...RUN_HISTORY_QUERY_ROOT, beforeId] as const,
  queryFn: ({ queryKey, signal }) =>
    resource.list({ beforeId: queryKey[RUN_HISTORY_CURSOR_INDEX] as number | undefined, signal }),
  enabled: computed(() => props.visible),
  getNextCursor: (page) => page.next_before_id,
})
const runs = computed(() => runPage.value?.runs ?? [])
const runDetail = useCreditRunDetail<AdminCreditRun>(resource.get)
const selectedRunId = runDetail.selectedRunId
const selectedRun = runDetail.run
const runDetailLoading = runDetail.loading
const runDetailError = runDetail.error
const currentPage = pagination.currentPage
const knownPageCount = pagination.knownPageCount
const canAdvance = pagination.canAdvance
const canRetreat = pagination.canRetreat
useFailureToast(initialError, feedback.runHistoryLoadFailed, {
  shouldFireOnMount: false,
  resolveError: () => runHistoryQuery.error.value,
})
useFailureToast(
  runDetailError,
  () => {
    feedback.runDetailFailed(undefined)
  },
  { shouldFireOnMount: false },
)

watch(
  () => props.visible,
  (visible) => {
    stopRefreshTimer()
    if (!visible) {
      pagination.reset()
      clearExpansion()
      void queryClient.removeQueries({ queryKey: [...RUN_HISTORY_QUERY_ROOT] })
      return
    }
    pagination.reset()
    clearExpansion()
    scheduleRefresh()
  },
)

onBeforeUnmount(() => {
  runDetail.close()
  stopRefreshTimer()
})

function refreshRuns() {
  // Same-key refetch: stale rows stay mounted with the table loading state.
  void refreshRunHistory()
}

// Polls run history while the dialog is visible — `useIntervalFn` owns the
// interval lifecycle (auto-paused on unmount), replacing manual setInterval pairing.
const { pause: stopRefreshTimer, resume: scheduleRefresh } = useIntervalFn(refreshRuns, 5_000, {
  immediate: false,
})

function expandRun(event: AdminTableRowExpandEvent<AdminCreditRun>) {
  expandedRows.value = { [String(event.data.id)]: true }
  void runDetail.open(event.data.id)
}

function collapseRun(event: AdminTableRowCollapseEvent) {
  if ((event.data as AdminCreditRun).id === selectedRunId.value) clearExpansion()
}

function nextPage() {
  if (!pagination.advance()) return
  clearExpansion()
}

function previousPage() {
  if (!pagination.retreat()) return
  clearExpansion()
}

function clearExpansion() {
  runDetail.close()
  expandedRows.value = {}
}
</script>
