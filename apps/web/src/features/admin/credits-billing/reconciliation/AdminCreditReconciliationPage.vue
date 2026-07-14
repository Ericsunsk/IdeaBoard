<template>
  <div class="flex min-w-0 flex-col gap-4">
    <section
      class="rounded-[var(--radius-ib-panel)] border border-ib-border-soft bg-ib-surface p-5"
    >
      <div class="flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between">
        <div class="min-w-0">
          <h2 class="m-0 text-admin-modal-title font-[780] text-admin-title-color">Idea对账</h2>
          <p class="mt-1 mb-0 text-admin-body text-admin-text-3">
            检查 Wallet、Ledger、Task Billing 与结算状态的一致性。异常仅报告和隔离，不执行自动修复。
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
          <AdminFormField
            label="Stale Hold 阈值（小时）"
            for="admin-credit-reconciliation-hours"
            hint="最小 1 小时"
          >
            <InputNumber
              v-model="staleHoldHours"
              input-id="admin-credit-reconciliation-hours"
              :min="1"
              :max="8760"
              :use-grouping="false"
              class="w-44"
            />
          </AdminFormField>
          <Button
            label="启动对账"
            :loading="startPending"
            :disabled="!canStart"
            @click="requestStart"
          />
        </div>
      </div>
      <Message class="mt-4" severity="warn" :closable="false">
        对账可能冻结高风险 Wallet。该动作不会覆盖余额、修改历史 Ledger 或自动修复异常。
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
              :total-pages="runHasNext ? runPage + 1 : runPage"
              :prev-disabled="runPage === 1 || runsLoading"
              :next-disabled="!runHasNext || runsLoading"
              @prev="previousRunPage"
              @next="nextRunPage"
            />
          </template>
        </AdminResourceToolbar>
      </template>

      <Message v-if="runsError" class="m-5" severity="error" :closable="false">
        对账运行记录加载失败，请刷新重试。
      </Message>
      <DataTableFrame
        v-else
        :value="runs"
        data-key="id"
        :loading="runsLoading"
        scrollable
        selection-mode="single"
        :selection="selectedRunRow"
        table-style="min-width: 1160px"
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
        <Column header="异常" style="width: 180px">
          <template #body="{ data }">
            <span>{{ data.finding_count }} 项</span>
            <span class="ml-2 text-admin-danger">{{ data.critical_count }} 严重</span>
          </template>
        </Column>
        <Column header="隔离 Wallet" field="isolated_wallet_count" style="width: 130px" />
        <Column header="阈值" style="width: 130px">
          <template #body="{ data }"
            >{{ Math.round(data.stale_hold_after_seconds / 3600) }} 小时</template
          >
        </Column>
        <Column header="发起人" style="width: 110px">
          <template #body="{ data }">User #{{ data.requested_by }}</template>
        </Column>
        <Column header="启动原因" style="min-width: 240px">
          <template #body="{ data }">
            <span class="block max-w-72 truncate" :title="data.reason">{{ data.reason }}</span>
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
      <div
        class="flex flex-col gap-3 border-b border-admin-border-soft px-5 py-4 lg:flex-row lg:items-center lg:justify-between"
      >
        <div class="min-w-0">
          <div class="flex flex-wrap items-center gap-2">
            <h3 class="m-0 text-admin-title-sm font-[760] text-admin-title-color">
              Run #{{ selectedRunId }}
            </h3>
            <Tag
              v-if="selectedRun"
              :value="getRunStatusLabel(selectedRun.status)"
              :severity="getRunStatusSeverity(selectedRun.status)"
              :class="adminCompactTagClass"
            />
          </div>
          <div v-if="selectedRun" class="mt-1 text-admin-caption text-admin-text-3">
            尝试 {{ selectedRun.attempt_count }} 次 · {{ selectedRun.finding_count }} 项异常 ·
            {{ selectedRun.isolated_wallet_count }} 个 Wallet 已隔离
          </div>
          <div
            v-if="selectedRun"
            class="mt-1 max-w-3xl break-words text-admin-caption text-admin-text-2"
          >
            <span class="font-[680]">启动原因：</span>{{ selectedRun.reason }}
          </div>
        </div>
        <div class="flex flex-wrap items-center gap-2">
          <Select
            v-model="findingStatusFilter"
            :options="findingStatusOptions"
            option-label="label"
            option-value="value"
            aria-label="异常状态筛选"
            class="w-36"
          />
          <AdminPagerControls
            :current-page="findingPage"
            :total-pages="findingHasNext ? findingPage + 1 : findingPage"
            :prev-disabled="findingPage === 1 || findingsLoading"
            :next-disabled="!findingHasNext || findingsLoading"
            @prev="previousFindingPage"
            @next="nextFindingPage"
          />
        </div>
      </div>

      <Message
        v-if="selectedRun?.error_message"
        class="mx-5 mt-4"
        severity="error"
        :closable="false"
      >
        {{ formatReconciliationRunError(selectedRun.error_message) }}
      </Message>
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
      <Message v-if="findingsError" class="m-5" severity="error" :closable="false">
        <div class="flex flex-wrap items-center justify-between gap-3">
          <span>异常列表加载失败。</span>
          <Button
            label="重试异常列表"
            text
            :loading="findingsLoading"
            @click="resource.findingsQuery.refetch()"
          />
        </div>
      </Message>
      <DataTableFrame
        v-else
        :value="findings"
        data-key="id"
        :loading="findingsLoading || runDetailLoading"
        scrollable
        table-style="min-width: 1120px"
      >
        <template #empty>
          <div class="py-12 text-center text-admin-body text-admin-text-3">当前筛选下暂无异常</div>
        </template>
        <Column header="异常" style="min-width: 230px">
          <template #body="{ data }">
            <div class="font-[700] text-admin-text-1">{{ data.finding_type }}</div>
            <div class="mt-1 flex gap-1.5">
              <Tag
                :value="getFindingSeverityLabel(data.severity)"
                :severity="getFindingSeverityTag(data.severity)"
                :class="adminCompactTagClass"
              />
              <Tag
                :value="data.risk === 'high' ? '高风险' : '低风险'"
                :severity="data.risk === 'high' ? 'danger' : 'secondary'"
                :class="adminCompactTagClass"
              />
            </div>
          </template>
        </Column>
        <Column header="关联对象" style="min-width: 250px">
          <template #body="{ data }">
            <div class="flex flex-wrap gap-1.5">
              <span
                v-if="data.wallet_user_id"
                class="rounded-[var(--radius-ib-pill)] bg-ib-surface-muted px-2 py-1 text-admin-caption font-[680]"
              >
                Wallet · User #{{ data.wallet_user_id }}
              </span>
              <span
                class="rounded-[var(--radius-ib-control)] border border-admin-border-soft px-2 py-1 text-admin-caption font-[680]"
              >
                {{ data.subject_type }} · {{ data.subject_id }}
              </span>
            </div>
            <div
              class="mt-1 max-w-96 truncate text-admin-caption text-admin-text-3"
              :title="formatFindingDetails(data.details)"
            >
              {{ formatFindingDetails(data.details) }}
            </div>
          </template>
        </Column>
        <Column header="隔离状态" style="width: 130px">
          <template #body="{ data }">
            <Tag
              :value="data.isolated ? '已隔离' : '未隔离'"
              :severity="data.isolated ? 'danger' : 'secondary'"
              :class="adminCompactTagClass"
            />
          </template>
        </Column>
        <Column header="处理状态" style="width: 120px">
          <template #body="{ data }">
            <Tag
              :value="data.status === 'open' ? '待处理' : '已解决'"
              :severity="data.status === 'open' ? 'warn' : 'success'"
              :class="adminCompactTagClass"
            />
          </template>
        </Column>
        <Column header="次数" style="width: 90px">
          <template #body="{ data }">{{ data.occurrence_count }}</template>
        </Column>
        <Column header="最近发现" style="width: 190px">
          <template #body="{ data }">{{ formatAdminDateTime(data.last_seen_at) }}</template>
        </Column>
      </DataTableFrame>
    </section>

    <div
      v-else
      class="flex min-h-52 flex-col items-center justify-center rounded-[var(--radius-ib-panel)] border border-dashed border-admin-border-soft bg-ib-surface text-center"
    >
      <div class="text-admin-title-sm font-[720] text-admin-title-color">选择一条 Run 查看异常</div>
      <div class="mt-1 text-admin-body text-admin-text-3">这里不会提供自动修复按钮。</div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { Refresh } from '@iconoir/vue'
import Button from 'primevue/button'
import Column from 'primevue/column'
import InputNumber from 'primevue/inputnumber'
import Message from 'primevue/message'
import Select from 'primevue/select'
import Tag from 'primevue/tag'
import AdminFormField from '../../shared/components/AdminFormField.vue'
import AdminPagerControls from '../../shared/components/AdminPagerControls.vue'
import AdminResourceToolbar from '../../shared/components/AdminResourceToolbar.vue'
import AdminTablePanel from '../../shared/components/AdminTablePanel.vue'
import { useAdminConfirm } from '../../shared/composables/useAdminConfirm'
import DataTableFrame from '../../../../shared/components/DataTableFrame.vue'
import { adminCompactTagClass, adminRoundIconButtonClass } from '../../shared/lib/adminUiClasses'
import AdminActionReasonField from '../AdminActionReasonField.vue'
import { validateAdminActionReason } from '../../shared/validation/adminActionReasonContract'
import { useCreditsBillingFeedback } from '../creditsBillingFeedback'
import {
  formatAdminDateTime,
  formatFindingDetails,
  formatReconciliationRunError,
  getFindingSeverityLabel,
  getFindingSeverityTag,
  getRunStatusLabel,
  getRunStatusSeverity,
} from '../creditsBillingFormat'
import {
  useAdminCreditReconciliationResource,
  type AdminCreditReconciliationRun,
} from './adminCreditReconciliationResource'

const staleHoldHours = ref<number | null>(24)
const startReason = ref('')
const reasonError = ref('')
const runBeforeId = ref<number>()
const runCursorStack = ref<Array<number | undefined>>([])
const selectedRunId = ref<number | null>(null)
const findingStatusFilter = ref<'all' | 'open' | 'resolved'>('all')
const findingBeforeId = ref<number>()
const findingCursorStack = ref<Array<number | undefined>>([])
const findingStatusOptions = [
  { label: '全部状态', value: 'all' },
  { label: '待处理', value: 'open' },
  { label: '已解决', value: 'resolved' },
] satisfies Array<{ label: string; value: 'all' | 'open' | 'resolved' }>

const confirm = useAdminConfirm()
const feedback = useCreditsBillingFeedback()
const resource = useAdminCreditReconciliationResource({
  runBeforeId,
  selectedRunId,
  findingStatus: computed(() =>
    findingStatusFilter.value === 'all' ? undefined : findingStatusFilter.value,
  ),
  findingBeforeId,
})
const { selectedRun, startPending } = resource
const runs = computed(() => resource.runs.value?.runs ?? [])
const findings = computed(() => resource.findings.value?.findings ?? [])
const selectedRunRow = computed(
  () => runs.value.find((run) => run.id === selectedRunId.value) ?? null,
)
const runPage = computed(() => runCursorStack.value.length + 1)
const findingPage = computed(() => findingCursorStack.value.length + 1)
const runHasNext = computed(() => Boolean(resource.runs.value?.next_before_id))
const findingHasNext = computed(() => Boolean(resource.findings.value?.next_before_id))
const canStart = computed(
  () =>
    staleHoldHours.value != null &&
    Number.isInteger(staleHoldHours.value) &&
    staleHoldHours.value >= 1 &&
    staleHoldHours.value <= 8760,
)
const runsLoading = computed(
  () => resource.runsQuery.isLoading.value || resource.runsQuery.isFetching.value,
)
const runDetailLoading = computed(
  () => resource.runQuery.isLoading.value || resource.runQuery.isFetching.value,
)
const findingsLoading = computed(
  () => resource.findingsQuery.isLoading.value || resource.findingsQuery.isFetching.value,
)
const runsError = computed(() => resource.runsQuery.error.value)
const runDetailError = computed(() => resource.runQuery.error.value)
const findingsError = computed(() => resource.findingsQuery.error.value)

watch(runsError, (error) => {
  if (error) feedback.reconciliationLoadFailed(error)
})
watch([runDetailError, findingsError], ([runError, listError]) => {
  const error = runError || listError
  if (error) feedback.reconciliationLoadFailed(error)
})
watch([selectedRunId, findingStatusFilter], () => {
  findingBeforeId.value = undefined
  findingCursorStack.value = []
})

function requestStart() {
  const hours = staleHoldHours.value
  if (hours == null || !Number.isInteger(hours) || hours < 1) return
  const reasonValidation = validateAdminActionReason(startReason.value)
  reasonError.value = reasonValidation.error
  if (!reasonValidation.valid) return

  const reason = reasonValidation.normalizedReason
  confirm.request({
    title: '确认启动Idea对账',
    message: `将扫描 Ledger 与 Task Billing，一旦发现高风险不一致可能隔离 Wallet。Stale Hold 阈值为 ${hours} 小时。`,
    confirmLabel: '启动对账',
    intent: 'warn',
    onConfirm: async () => {
      try {
        const response = await resource.start({
          stale_hold_after_seconds: hours * 3600,
          reason,
        })
        selectedRunId.value = response.data?.id ?? null
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
  const next = resource.runs.value?.next_before_id
  if (!next) return
  runCursorStack.value = [...runCursorStack.value, runBeforeId.value]
  runBeforeId.value = next
}

function previousRunPage() {
  const stack = [...runCursorStack.value]
  runBeforeId.value = stack.pop()
  runCursorStack.value = stack
}

function nextFindingPage() {
  const next = resource.findings.value?.next_before_id
  if (!next) return
  findingCursorStack.value = [...findingCursorStack.value, findingBeforeId.value]
  findingBeforeId.value = next
}

function previousFindingPage() {
  const stack = [...findingCursorStack.value]
  findingBeforeId.value = stack.pop()
  findingCursorStack.value = stack
}
</script>
