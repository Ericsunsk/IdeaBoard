<template>
  <AdminDialogShell
    :visible="visible"
    header="检查记录"
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
            <Button
              text
              :class="iconButtonClass"
              aria-label="刷新检查记录"
              :loading="runsLoading"
              @click="refreshRuns"
            >
              <template #icon><Refresh class="size-4" /></template>
            </Button>
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

      <Message v-if="backgroundError" class="m-4" severity="secondary" :closable="false">
        <div class="flex flex-wrap items-center justify-between gap-3">
          <span>检查记录刷新失败，当前显示上次加载的数据。</span>
          <Button
            label="重试刷新"
            severity="secondary"
            size="small"
            text
            :class="adminTextButtonClass"
            :loading="runsLoading"
            @click="refreshRuns"
          />
        </div>
      </Message>

      <Message v-if="initialError" class="m-5" severity="error" :closable="false">
        <div class="flex flex-wrap items-center justify-between gap-3">
          <span>检查记录加载失败。</span>
          <Button
            label="重新加载检查记录"
            :class="adminActionButtonClass"
            severity="secondary"
            outlined
            :loading="runsLoading"
            @click="loadRuns()"
          />
        </div>
      </Message>

      <DataTableFrame
        v-else
        v-model:expanded-rows="expandedRows"
        :value="runs"
        data-key="id"
        :loading="runsLoading"
        scrollable
        table-style="min-width: 820px"
        table-aria-label="账本检查记录"
        scroll-aria-label="横向滚动查看账本检查记录"
        @row-expand="expandRun"
        @row-collapse="collapseRun"
      >
        <template #empty>
          <div
            class="flex min-h-40 flex-col items-center justify-center px-6 py-10 text-center"
            role="status"
          >
            <span class="font-semibold">暂无检查记录</span>
            <span class="mt-1 text-admin-body text-admin-text-3">
              运行一次检查后，记录会显示在这里
            </span>
          </div>
        </template>

        <Column expander style="width: 52px" />
        <Column header="时间" style="width: 190px">
          <template #body="{ data }">
            <span class="whitespace-nowrap">{{ formatAdminDateTime(data.requested_at) }}</span>
          </template>
        </Column>
        <Column header="状态" style="width: 110px">
          <template #body="{ data }">
            <Tag
              :value="getRunStatusLabel(data.status)"
              :severity="getRunStatusSeverity(data.status)"
              class="text-admin-caption"
            />
          </template>
        </Column>
        <Column header="Trigger" style="width: 120px">
          <template #body="{ data }">
            {{ getReconciliationTriggerLabel(data) }}
          </template>
        </Column>
        <Column header="结果" style="min-width: 240px">
          <template #body="{ data }">{{ formatReconciliationRunResult(data) }}</template>
        </Column>
        <Column header="耗时" style="width: 120px">
          <template #body="{ data }">{{ formatReconciliationRunDuration(data) }}</template>
        </Column>

        <template #expansion="{ data }">
          <div class="border-y border-admin-border-soft bg-ib-surface-muted px-5 py-4">
            <div
              v-if="runDetailLoading && selectedRunId === data.id"
              class="flex min-h-24 items-center justify-center"
              aria-label="加载 Run 详情"
            >
              <ProgressSpinner class="h-7 w-7 text-admin-accent" stroke-width="4" />
            </div>
            <Message
              v-else-if="runDetailError && selectedRunId === data.id"
              severity="error"
              :closable="false"
            >
              <div class="flex flex-wrap items-center justify-between gap-3">
                <span>Run 详情加载失败。</span>
                <Button
                  label="重试 Run 详情"
                  text
                  :class="adminTextButtonClass"
                  :loading="runDetailLoading"
                  @click="runDetail.refresh()"
                />
              </div>
            </Message>
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
      </DataTableFrame>
    </AdminTablePanel>

    <template #footer>
      <Button
        label="关闭"
        :class="adminActionButtonClass"
        severity="secondary"
        outlined
        @click="$emit('update:visible', false)"
      />
    </template>
  </AdminDialogShell>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { Refresh } from '@iconoir/vue'
import Button from 'primevue/button'
import Column from 'primevue/column'
import Message from 'primevue/message'
import ProgressSpinner from 'primevue/progressspinner'
import Tag from 'primevue/tag'
import type { DataTableRowCollapseEvent, DataTableRowExpandEvent } from 'primevue/datatable'
import DataTableFrame from '../../../../shared/components/DataTableFrame.vue'
import AdminDialogShell from '../../shared/components/AdminDialogShell.vue'
import AdminPagerControls from '../../shared/components/AdminPagerControls.vue'
import AdminResourceToolbar from '../../shared/components/AdminResourceToolbar.vue'
import AdminTablePanel from '../../shared/components/AdminTablePanel.vue'
import { useAdminCollectionState } from '../../shared/composables/useAdminCollectionState'
import { useAdminCursorPagination } from '../../shared/composables/useAdminCursorPagination'
import {
  formatAdminDateTime,
  formatReconciliationRunError,
  getRunStatusLabel,
  getRunStatusSeverity,
} from '../creditsBillingFormat'
import { adminActionButtonClass, adminTextButtonClass } from '../../shared/lib/adminUiClasses'
import {
  createAdminCreditRunHistoryResource,
  type AdminCreditRun,
} from './adminCreditRunHistoryResource'
import {
  formatReconciliationRunDuration,
  formatReconciliationRunResult,
  getReconciliationTriggerLabel,
} from './reconciliationRunPresentation'
import { useAdminCreditRunDetail } from './useAdminCreditRunDetail'

const props = defineProps<{ visible: boolean }>()
defineEmits<{ 'update:visible': [visible: boolean] }>()

const expandedRows = ref<Record<string, boolean>>({})
const resource = createAdminCreditRunHistoryResource()
let pagination: ReturnType<typeof useAdminCursorPagination<number>>
const collection = useAdminCollectionState({
  load: (signal) => resource.list({ beforeId: pagination.cursor.value, signal }),
  isEmpty: (page) => page.runs.length === 0,
})
const { data: runPage, loading: runsLoading, initialError, backgroundError } = collection
pagination = useAdminCursorPagination<number>({ pending: runsLoading })
const runs = computed(() => runPage.value?.runs ?? [])
const runDetail = useAdminCreditRunDetail<AdminCreditRun>(resource.get)
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
      collection.reset()
      clearExpansion()
      return
    }
    pagination.reset()
    clearExpansion()
    void loadRuns()
    scheduleRefresh()
  },
)

onBeforeUnmount(() => {
  collection.cancel()
  runDetail.close()
  stopRefreshTimer()
})

async function loadRuns({ preserveData = false }: { preserveData?: boolean } = {}) {
  const page = await collection.reload({ preserveData })
  if (page) pagination.setNextCursor(page.next_before_id)
}

function refreshRuns() {
  void loadRuns({ preserveData: true })
}

let refreshTimer: number | undefined

function scheduleRefresh() {
  refreshTimer = window.setInterval(refreshRuns, 5_000)
}

function stopRefreshTimer() {
  if (refreshTimer === undefined) return
  window.clearInterval(refreshTimer)
  refreshTimer = undefined
}

function expandRun(event: DataTableRowExpandEvent<AdminCreditRun>) {
  expandedRows.value = { [String(event.data.id)]: true }
  void runDetail.open(event.data.id)
}

function collapseRun(event: DataTableRowCollapseEvent) {
  if ((event.data as AdminCreditRun).id === selectedRunId.value) clearExpansion()
}

function nextPage() {
  if (!pagination.advance()) return
  clearExpansion()
  void loadRuns()
}

function previousPage() {
  if (!pagination.retreat()) return
  clearExpansion()
  void loadRuns()
}

function clearExpansion() {
  runDetail.close()
  expandedRows.value = {}
}
</script>
