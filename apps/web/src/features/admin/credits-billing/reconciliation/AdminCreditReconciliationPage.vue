<template>
  <div class="flex min-w-0 flex-col gap-4">
    <section
      class="rounded-[var(--radius-ib-panel)] border border-ib-border-soft bg-ib-surface p-5"
    >
      <div class="flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between">
        <div class="min-w-0">
          <h2 class="m-0 text-admin-modal-title font-[780] text-admin-title-color">Idea对账</h2>
          <p class="mt-1 mb-0 text-admin-body text-admin-text-3">
            检查 Wallet、Ledger、Task Billing 与结算状态的一致性。异常统一在账本健康页处理。
          </p>
        </div>
        <div class="flex flex-wrap items-end gap-2">
          <AdminActionReasonField
            v-model="startReason"
            input-id="admin-credit-reconciliation-reason"
            label="启动原因"
            :error="reasonError"
            hint="必填，1–1000 个字符，用于审计追溯"
            :rows="2"
            body-class="flex min-w-0 w-72 flex-col gap-1.5"
            @update:model-value="reasonError = ''"
          />
          <Button label="启动对账" :loading="startPending" @click="requestStart" />
        </div>
      </div>
      <Message class="mt-4" severity="warn" :closable="false">
        Stale Hold 固定按 24 小时检查。高风险异常会限制消费，但不会覆盖余额或自动修复。
      </Message>
    </section>

    <AdminTablePanel>
      <template #toolbar>
        <AdminResourceToolbar>
          <template #start>
            <div>
              <div class="text-admin-title-sm font-[740] text-admin-title-color">对账运行记录</div>
              <div class="mt-0.5 text-admin-caption text-admin-text-3">
                按 Run ID 倒序，执行中每 5 秒刷新
              </div>
            </div>
          </template>
          <template #actions>
            <Button
              aria-label="刷新对账运行记录"
              text
              :class="adminRoundIconButtonClass"
              :loading="runsLoading"
              @click="resource.runsQuery.refetch()"
            >
              <template #icon><Refresh class="size-4" /></template>
            </Button>
          </template>
          <template #pager>
            <AdminPagerControls
              :current-page="runPage"
              :total-pages="runKnownPageCount"
              :prev-disabled="!runCanRetreat"
              :next-disabled="!runCanAdvance"
              @prev="previousRunPage"
              @next="nextRunPage"
            />
          </template>
        </AdminResourceToolbar>
      </template>

      <Message v-if="runsError" class="m-5" severity="error" :closable="false">
        <div class="flex flex-wrap items-center justify-between gap-3">
          <span>对账运行记录加载失败。</span>
          <Button
            label="重新加载运行记录"
            severity="secondary"
            outlined
            :loading="runsLoading"
            @click="resource.runsQuery.refetch()"
          />
        </div>
      </Message>
      <DataTableFrame
        v-else
        :value="runs"
        data-key="id"
        :loading="runsLoading"
        scrollable
        selection-mode="single"
        :selection="selectedRunRow"
        table-style="min-width: 1100px"
        @update:selection="selectRun"
      >
        <template #empty>
          <div class="py-12 text-center text-admin-body text-admin-text-3">暂无对账记录</div>
        </template>
        <Column field="id" header="Run" style="width: 90px">
          <template #body="{ data }"
            ><span class="font-[720]">#{{ data.id }}</span></template
          >
        </Column>
        <Column header="状态" style="width: 110px">
          <template #body="{ data }">
            <Tag
              :value="getRunStatusLabel(data.status)"
              :severity="getRunStatusSeverity(data.status)"
              :class="adminCompactTagClass"
            />
          </template>
        </Column>
        <Column header="结果" style="width: 210px">
          <template #body="{ data }">
            <span>{{ data.finding_count }} 项异常</span>
            <span class="ml-2 text-admin-danger">{{ data.critical_count }} 严重</span>
          </template>
        </Column>
        <Column header="受限 Wallet" field="isolated_wallet_count" style="width: 130px" />
        <Column header="触发方式" style="width: 120px">
          <template #body="{ data }">
            <Tag
              :value="data.trigger === 'scheduled' ? '自动调度' : '人工触发'"
              :severity="data.trigger === 'scheduled' ? 'info' : 'secondary'"
              :class="adminCompactTagClass"
            />
          </template>
        </Column>
        <Column header="发起人" style="width: 110px">
          <template #body="{ data }">
            {{ data.requested_by ? `User #${data.requested_by}` : '系统' }}
          </template>
        </Column>
        <Column header="启动原因" style="min-width: 240px">
          <template #body="{ data }">
            <span class="block max-w-72 truncate" :title="data.reason || ''">
              {{ data.reason || '每日账本健康检查' }}
            </span>
          </template>
        </Column>
        <Column header="发起时间" style="width: 190px">
          <template #body="{ data }">{{ formatAdminDateTime(data.requested_at) }}</template>
        </Column>
      </DataTableFrame>
    </AdminTablePanel>

    <section
      v-if="selectedRunId"
      class="min-w-0 rounded-[var(--radius-ib-panel)] border border-ib-border-soft bg-ib-surface"
    >
      <Message v-if="runDetailError" class="m-5" severity="error" :closable="false">
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
      <template v-else-if="selectedRun">
        <div
          class="flex flex-col gap-3 border-b border-admin-border-soft px-5 py-4 lg:flex-row lg:items-center lg:justify-between"
        >
          <div class="min-w-0">
            <div class="flex flex-wrap items-center gap-2">
              <h3 class="m-0 text-admin-title-sm font-[760] text-admin-title-color">
                Run #{{ selectedRun.id }}
              </h3>
              <Tag
                :value="getRunStatusLabel(selectedRun.status)"
                :severity="getRunStatusSeverity(selectedRun.status)"
                :class="adminCompactTagClass"
              />
            </div>
            <div class="mt-1 text-admin-caption text-admin-text-3">
              尝试 {{ selectedRun.attempt_count }} 次 · {{ selectedRun.finding_count }} 项异常 ·
              {{ selectedRun.isolated_wallet_count }} 个 Wallet 受限
            </div>
          </div>
          <div class="text-admin-caption text-admin-text-3">
            {{ selectedRun.trigger === 'scheduled' ? '自动调度' : '人工触发' }}
          </div>
        </div>

        <Message
          v-if="selectedRun.error_message"
          class="mx-5 mt-4"
          severity="error"
          :closable="false"
        >
          {{ formatReconciliationRunError(selectedRun.error_message) }}
        </Message>

        <dl class="grid gap-x-6 gap-y-4 px-5 py-5 sm:grid-cols-2 xl:grid-cols-4">
          <div>
            <dt class="text-admin-caption font-[700] text-admin-text-3">发起时间</dt>
            <dd class="mt-1 mb-0 text-admin-body text-admin-text-1">
              {{ formatAdminDateTime(selectedRun.requested_at) }}
            </dd>
          </div>
          <div>
            <dt class="text-admin-caption font-[700] text-admin-text-3">开始时间</dt>
            <dd class="mt-1 mb-0 text-admin-body text-admin-text-1">
              {{ formatAdminDateTime(selectedRun.started_at) }}
            </dd>
          </div>
          <div>
            <dt class="text-admin-caption font-[700] text-admin-text-3">完成时间</dt>
            <dd class="mt-1 mb-0 text-admin-body text-admin-text-1">
              {{ formatAdminDateTime(selectedRun.completed_at) }}
            </dd>
          </div>
          <div>
            <dt class="text-admin-caption font-[700] text-admin-text-3">计划时间</dt>
            <dd class="mt-1 mb-0 text-admin-body text-admin-text-1">
              {{ formatAdminDateTime(selectedRun.scheduled_for) }}
            </dd>
          </div>
          <div class="sm:col-span-2 xl:col-span-4">
            <dt class="text-admin-caption font-[700] text-admin-text-3">审计原因</dt>
            <dd class="mt-1 mb-0 break-words text-admin-body text-admin-text-1">
              {{ selectedRun.reason || '每日账本健康检查' }}
            </dd>
          </div>
        </dl>
      </template>
    </section>

    <div
      v-else
      class="flex min-h-40 items-center justify-center rounded-[var(--radius-ib-panel)] border border-dashed border-admin-border-soft bg-ib-surface text-admin-body text-admin-text-3"
    >
      选择一条 Run 查看运行详情
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { Refresh } from '@iconoir/vue'
import Button from 'primevue/button'
import Column from 'primevue/column'
import Message from 'primevue/message'
import Tag from 'primevue/tag'
import AdminPagerControls from '../../shared/components/AdminPagerControls.vue'
import AdminResourceToolbar from '../../shared/components/AdminResourceToolbar.vue'
import AdminTablePanel from '../../shared/components/AdminTablePanel.vue'
import { useAdminConfirm } from '../../shared/composables/useAdminConfirm'
import { useAdminCursorPagination } from '../../shared/composables/useAdminCursorPagination'
import DataTableFrame from '../../../../shared/components/DataTableFrame.vue'
import { adminCompactTagClass, adminRoundIconButtonClass } from '../../shared/lib/adminUiClasses'
import AdminActionReasonField from '../AdminActionReasonField.vue'
import { validateAdminActionReason } from '../../shared/validation/adminActionReasonContract'
import { useAdminCreditsBillingOperationsFeedback } from '../creditsBillingFeedback'
import {
  formatAdminDateTime,
  formatReconciliationRunError,
  getRunStatusLabel,
  getRunStatusSeverity,
} from '../creditsBillingFormat'
import {
  useAdminCreditReconciliationResource,
  type AdminCreditReconciliationRun,
} from './adminCreditReconciliationResource'

const startReason = ref('')
const reasonError = ref('')
const selectedRunId = ref<number | null>(null)
const confirm = useAdminConfirm()
const feedback = useAdminCreditsBillingOperationsFeedback()
const runPagination = useAdminCursorPagination<number>({
  pending: () => resource.runsQuery.isLoading.value || resource.runsQuery.isFetching.value,
})
const runBeforeId = runPagination.cursor
const resource = useAdminCreditReconciliationResource({ runBeforeId, selectedRunId })
const { selectedRun, startPending } = resource
const runs = computed(() => resource.runs.value?.runs ?? [])
const selectedRunRow = computed(
  () => runs.value.find((run) => run.id === selectedRunId.value) ?? null,
)
const runPage = runPagination.currentPage
const runKnownPageCount = runPagination.knownPageCount
const runCanAdvance = runPagination.canAdvance
const runCanRetreat = runPagination.canRetreat
const runsLoading = computed(
  () => resource.runsQuery.isLoading.value || resource.runsQuery.isFetching.value,
)
const runDetailLoading = computed(
  () => resource.runQuery.isLoading.value || resource.runQuery.isFetching.value,
)
const runsError = computed(() => resource.runsQuery.error.value)
const runDetailError = computed(() => resource.runQuery.error.value)

watch(runsError, (error) => {
  if (error) feedback.reconciliationLoadFailed(error)
})
watch(runDetailError, (error) => {
  if (error) feedback.reconciliationLoadFailed(error)
})
watch(
  [
    () => resource.runsQuery.dataUpdatedAt.value,
    () => resource.runsQuery.isFetching.value,
    () => resource.runsQuery.isPlaceholderData.value,
    () => resource.runsDataCriteria.value,
    () => runBeforeId.value,
  ],
  () => {
    const criteria = resource.runsDataCriteria.value
    if (
      resource.runsQuery.isFetching.value ||
      resource.runsQuery.isPlaceholderData.value ||
      !criteria ||
      criteria.beforeId !== runBeforeId.value
    ) {
      return
    }
    runPagination.setNextCursor(resource.runs.value?.next_before_id)
  },
  { immediate: true },
)

function requestStart() {
  const reasonValidation = validateAdminActionReason(startReason.value)
  reasonError.value = reasonValidation.error
  if (!reasonValidation.valid) return

  const reason = reasonValidation.normalizedReason
  confirm.request({
    title: '确认启动Idea对账',
    message:
      '将扫描 Ledger 与 Task Billing，一旦发现高风险不一致可能限制 Wallet。Stale Hold 固定按 24 小时检查。',
    confirmLabel: '启动对账',
    intent: 'warn',
    onConfirm: async () => {
      try {
        const response = await resource.start({ reason })
        selectedRunId.value = response.id
        startReason.value = ''
        feedback.reconciliationStarted()
      } catch (error) {
        feedback.reconciliationStartFailed(error)
      }
    },
  })
}

function selectRun(run: AdminCreditReconciliationRun | null) {
  selectedRunId.value = run?.id ?? null
}

function nextRunPage() {
  if (!runPagination.advance()) return
  selectedRunId.value = null
}

function previousRunPage() {
  if (!runPagination.retreat()) return
  selectedRunId.value = null
}
</script>
