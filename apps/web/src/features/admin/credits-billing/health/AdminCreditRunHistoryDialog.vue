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
              <div class="mt-0.5 text-admin-caption text-admin-text-3">
                检查发现包含仅报告的业务追踪异常；只有资金风险才会限制 Wallet
              </div>
            </div>
          </template>
          <template #actions>
            <Button
              label="刷新"
              severity="secondary"
              size="small"
              text
              aria-label="刷新检查记录"
              :loading="runsLoading"
              @click="resource.runsQuery.refetch()"
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
            :loading="runsLoading"
            @click="resource.runsQuery.refetch()"
          />
        </div>
      </Message>

      <Message v-if="initialError" class="m-5" severity="error" :closable="false">
        <div class="flex flex-wrap items-center justify-between gap-3">
          <span>检查记录加载失败。</span>
          <Button
            label="重新加载检查记录"
            severity="secondary"
            outlined
            :loading="runsLoading"
            @click="resource.runsQuery.refetch()"
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
                  :loading="runDetailLoading"
                  @click="resource.runQuery.refetch()"
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
      <Button label="关闭" severity="secondary" outlined @click="$emit('update:visible', false)" />
    </template>
  </AdminDialogShell>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
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
import { useAdminCursorPagination } from '../../shared/composables/useAdminCursorPagination'
import {
  formatAdminDateTime,
  formatReconciliationRunError,
  getRunStatusLabel,
  getRunStatusSeverity,
} from '../creditsBillingFormat'
import {
  useAdminCreditRunHistoryResource,
  type AdminCreditRun,
} from './adminCreditRunHistoryResource'
import {
  formatReconciliationRunDuration,
  formatReconciliationRunResult,
  getReconciliationTriggerLabel,
} from './reconciliationRunPresentation'

const props = defineProps<{ visible: boolean }>()
defineEmits<{ 'update:visible': [visible: boolean] }>()

const selectedRunId = ref<number | null>(null)
const expandedRows = ref<Record<string, boolean>>({})
const pagination = useAdminCursorPagination<number>({
  pending: () => resource.runsQuery.isLoading.value || resource.runsQuery.isFetching.value,
})
const beforeId = pagination.cursor
const resource = useAdminCreditRunHistoryResource({
  enabled: () => props.visible,
  beforeId,
  selectedRunId,
})
const runs = computed(() => resource.page.value?.runs ?? [])
const selectedRun = computed(() => {
  const run = resource.selectedRun.value
  return run?.id === selectedRunId.value ? run : undefined
})
const runsLoading = computed(
  () => resource.runsQuery.isLoading.value || resource.runsQuery.isFetching.value,
)
const runDetailLoading = computed(
  () => resource.runQuery.isLoading.value || resource.runQuery.isFetching.value,
)
const initialError = computed(() => resource.runsQuery.isError.value && !resource.page.value)
const backgroundError = computed(
  () => resource.runsQuery.isError.value && Boolean(resource.page.value),
)
const runDetailError = computed(() => resource.runQuery.isError.value)
const currentPage = pagination.currentPage
const knownPageCount = pagination.knownPageCount
const canAdvance = pagination.canAdvance
const canRetreat = pagination.canRetreat

watch(
  [
    () => resource.runsQuery.dataUpdatedAt.value,
    () => resource.runsQuery.isFetching.value,
    () => resource.runsQuery.isPlaceholderData.value,
    () => resource.dataCriteria.value,
    () => beforeId.value,
  ],
  () => {
    const criteria = resource.dataCriteria.value
    if (
      resource.runsQuery.isFetching.value ||
      resource.runsQuery.isPlaceholderData.value ||
      !criteria ||
      criteria.beforeId !== beforeId.value
    ) {
      return
    }
    pagination.setNextCursor(resource.page.value?.next_before_id)
  },
  { immediate: true },
)

watch(
  () => props.visible,
  (visible) => {
    if (!visible) return
    pagination.reset()
    clearExpansion()
  },
)

function expandRun(event: DataTableRowExpandEvent<AdminCreditRun>) {
  selectedRunId.value = event.data.id
  expandedRows.value = { [String(event.data.id)]: true }
}

function collapseRun(event: DataTableRowCollapseEvent) {
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
  selectedRunId.value = null
  expandedRows.value = {}
}
</script>
