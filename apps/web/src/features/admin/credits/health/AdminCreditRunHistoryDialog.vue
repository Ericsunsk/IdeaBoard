<template>
  <AdminDialogShell
    :visible="visible"
    header="检查记录"
    description="查看账本检查的历史运行记录。"
    size="wide"
    content-layout="scrollable"
    @update:visible="$emit('update:visible', $event)"
  >
    <AdminTablePanel>
      <template #toolbar>
        <AdminResourceToolbar>
          <template #start>
            <div class="min-w-0">
              <div class="text-admin-title-sm font-[740] text-admin-title-color">账本检查历史</div>
            </div>
          </template>
          <template #actions="{ iconButtonClass }">
            <AdminButton
              variant="ghost"
              :class="iconButtonClass"
              aria-label="刷新检查记录"
              :loading="runsLoading"
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
          <span>检查记录刷新失败，当前显示上次加载的数据。</span>
          <AdminButton
            label="重试刷新"
            severity="secondary"
            size="small"
            variant="ghost"
            :class="adminTextButtonClass"
            :loading="runsLoading"
            @click="refreshRuns"
          />
        </div>
      </AdminAlert>

      <AdminAlert v-if="initialError" class="m-5" severity="error">
        <div class="flex flex-wrap items-center justify-between gap-3">
          <span>检查记录加载失败。</span>
          <AdminButton
            label="重新加载检查记录"
            :class="adminActionButtonClass"
            severity="secondary"
            variant="outline"
            :loading="runsLoading"
            @click="refreshRuns"
          />
        </div>
      </AdminAlert>

      <AdminTable
        v-else
        v-model:expanded-rows="expandedRows"
        :value="runs"
        data-key="id"
        :loading="runsLoading"
        scrollable
        :frame-class="adminTableFrameClass"
        :table-style="{ minWidth: '820px' }"
        :class="adminTableClass"
        table-aria-label="账本检查记录"
        scroll-aria-label="横向滚动查看账本检查记录"
        @row-expand="expandRun"
        @row-collapse="collapseRun"
      >
        <template #empty>
          <AdminEmptyState title="暂无检查记录" description="运行一次检查后，记录会显示在这里" />
        </template>

        <AdminColumn expander style="width: 52px" />
        <AdminColumn header="时间" style="width: 190px">
          <template #body="{ data }">
            <span class="whitespace-nowrap">{{
              formatAdminDateTimeWithSeconds(data.requested_at)
            }}</span>
          </template>
        </AdminColumn>
        <AdminColumn header="状态" style="width: 110px">
          <template #body="{ data }">
            <AdminBadge
              :value="getRunStatusLabel(data.status)"
              :severity="getRunStatusSeverity(data.status)"
              class="text-admin-caption"
            />
          </template>
        </AdminColumn>
        <AdminColumn header="Trigger" style="width: 120px">
          <template #body="{ data }">
            {{ getReconciliationTriggerLabel(data) }}
          </template>
        </AdminColumn>
        <AdminColumn header="结果" style="min-width: 240px">
          <template #body="{ data }">{{ formatReconciliationRunResult(data) }}</template>
        </AdminColumn>
        <AdminColumn header="耗时" style="width: 120px">
          <template #body="{ data }">{{ formatReconciliationRunDuration(data) }}</template>
        </AdminColumn>

        <template #expansion="{ data }">
          <div class="border-y border-admin-border-soft bg-ib-surface-muted px-5 py-4">
            <div
              v-if="runDetailLoading && selectedRunId === data.id"
              class="flex min-h-24 items-center justify-center"
              aria-label="加载 Run 详情"
            >
              <AdminSpinner class="h-7 w-7" aria-label="加载 Run 详情" />
            </div>
            <AdminAlert v-else-if="runDetailError && selectedRunId === data.id" severity="error">
              <div class="flex flex-wrap items-center justify-between gap-3">
                <span>Run 详情加载失败。</span>
                <AdminButton
                  label="重试 Run 详情"
                  variant="ghost"
                  :class="adminTextButtonClass"
                  :loading="runDetailLoading"
                  @click="runDetail.refresh()"
                />
              </div>
            </AdminAlert>
            <dl
              v-else-if="selectedRunId === data.id && selectedRun"
              class="grid gap-x-6 gap-y-4 sm:grid-cols-2 xl:grid-cols-4"
            >
              <div>
                <dt class="text-admin-caption font-[700] text-admin-text-3">技术 ID</dt>
                <dd class="mt-1 mb-0 font-mono text-admin-body text-admin-text-1">
                  Run #{{ selectedRun.id }}
                </dd>
              </div>
              <div>
                <dt class="text-admin-caption font-[700] text-admin-text-3">尝试次数</dt>
                <dd class="mt-1 mb-0 text-admin-body text-admin-text-1">
                  {{ selectedRun.attempt_count }}
                </dd>
              </div>
              <div class="sm:col-span-2">
                <dt class="text-admin-caption font-[700] text-admin-text-3">审计原因</dt>
                <dd class="mt-1 mb-0 break-words text-admin-body text-admin-text-1">
                  {{ selectedRun.reason || '每日账本健康检查' }}
                </dd>
              </div>
              <div v-if="selectedRun.error_message" class="sm:col-span-2 xl:col-span-4">
                <dt class="text-admin-caption font-[700] text-admin-text-3">错误信息</dt>
                <dd class="mt-1 mb-0 break-words text-admin-body text-admin-danger">
                  {{ formatReconciliationRunError(selectedRun.error_message) }}
                </dd>
              </div>
            </dl>
          </div>
        </template>
      </AdminTable>
    </AdminTablePanel>

    <template #footer>
      <AdminButton
        variant="outline"
        :class="adminActionButtonClass"
        @click="$emit('update:visible', false)"
      >
        关闭
      </AdminButton>
    </template>
  </AdminDialogShell>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useQueryClient } from '@tanstack/vue-query'
import { useIntervalFn } from '@vueuse/core'
import { Refresh } from '@iconoir/vue'
import AdminAlert from '../../shared/components/AdminAlert.vue'
import AdminBadge from '../../shared/components/AdminBadge.vue'
import AdminButton from '../../shared/components/AdminButton.vue'
import AdminColumn from '../../shared/components/AdminColumn.vue'
import AdminSpinner from '../../shared/components/AdminSpinner.vue'
import AdminTable from '../../shared/components/AdminTable.vue'
import type {
  AdminTableRowCollapseEvent,
  AdminTableRowExpandEvent,
} from '../../shared/components/AdminTable.vue'
import AdminDialogShell from '../../shared/components/AdminDialogShell.vue'
import AdminEmptyState from '../../shared/components/AdminEmptyState.vue'
import AdminPagerControls from '../../shared/components/AdminPagerControls.vue'
import AdminResourceToolbar from '../../shared/components/AdminResourceToolbar.vue'
import AdminTablePanel from '../../shared/components/AdminTablePanel.vue'
import { useAdminCursorCollection } from '../../shared/composables/useAdminCursorCollection'
import {
  formatAdminDateTimeWithSeconds,
  formatReconciliationRunError,
  getRunStatusLabel,
  getRunStatusSeverity,
} from '../shared/format'
import {
  adminActionButtonClass,
  adminTableClass,
  adminTableFrameClass,
  adminTextButtonClass,
} from '../../shared/lib/uiClasses'
import { createAdminCreditRunHistoryResource, type AdminCreditRun } from './runHistoryResource'
import {
  formatReconciliationRunDuration,
  formatReconciliationRunResult,
  getReconciliationTriggerLabel,
} from './reconciliationRunPresentation'
import { useCreditRunDetail } from './useCreditRunDetail'

const props = defineProps<{ visible: boolean }>()
defineEmits<{ 'update:visible': [visible: boolean] }>()

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
