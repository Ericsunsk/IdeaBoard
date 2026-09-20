<template>
  <AdminDialogShell
    :visible="visible"
    size="slim"
    content-layout="scrollable"
    aria-label="任务详情控制台"
    description="查看任务执行的完整快照与结果。"
    @update:visible="handleVisibleUpdate"
  >
    <template #header>
      <div class="flex w-full items-center justify-between gap-3 pr-6">
        <div class="flex min-w-0 items-center gap-2.5">
          <span
            class="inline-flex size-7 shrink-0 items-center justify-center rounded-lg border border-admin-border-soft bg-admin-surface-muted"
            :style="{ color: getTaskStatusDefinition(detailTask?.status || '').color }"
          >
            <CheckCircle v-if="detailTask?.status === adminTaskStatuses.completed" class="size-3.5" />
            <XmarkCircle
              v-else-if="
                detailTask?.status === adminTaskStatuses.failed ||
                detailTask?.status === adminTaskStatuses.cancelled
              "
              class="size-3.5"
            />
            <Clock v-else class="size-3.5" />
          </span>
          <div class="flex min-w-0 flex-col">
            <DialogTitle class="text-sm font-semibold text-admin-text-1">
              任务详情控制台
            </DialogTitle>
            <div class="flex items-center gap-1.5 font-mono text-xs text-admin-text-3">
              <span class="truncate">ID: {{ detailTask?.task_id || taskId || '-' }}</span>
              <button
                v-if="detailTask?.task_id || taskId"
                type="button"
                class="flex size-5 cursor-pointer items-center justify-center rounded text-admin-text-3 transition-colors hover:bg-admin-surface-muted hover:text-admin-text-1"
                aria-label="复制任务 ID"
                @click="copyToClipboard(detailTask?.task_id || taskId || '')"
              >
                <Copy class="size-3" />
              </button>
            </div>
          </div>
        </div>

        <div class="flex shrink-0 items-center gap-2">
          <AdminBadge
            v-if="detailTask"
            :value="getTaskStatusDefinition(detailTask.status).text"
            :severity="getTaskStatusDefinition(detailTask.status).severity"
            rounded
          />
        </div>
      </div>
    </template>

    <div
      v-if="loading"
      class="flex min-h-40 items-center justify-center rounded-xl border border-admin-border-soft bg-admin-surface-muted/60 text-xs text-admin-text-3"
      role="status"
    >
      正在加载任务详情…
    </div>

    <div
      v-else-if="errorMessage"
      class="flex min-h-40 flex-col items-center justify-center gap-2.5 rounded-xl border border-admin-danger/20 bg-admin-danger/5 px-4 text-center"
      role="alert"
    >
      <span class="text-xs font-semibold text-admin-danger">{{ errorMessage }}</span>
      <AdminButton
        :class="adminActionButtonClass"
        size="small"
        variant="outline"
        @click="emit('retry')"
      >
        重试
      </AdminButton>
    </div>

    <div
      v-else-if="!detailTask"
      class="flex min-h-40 items-center justify-center rounded-xl border border-admin-border-soft bg-admin-surface-muted/60 text-xs text-admin-text-3"
    >
      任务详情不可用
    </div>

    <!-- 主信息单页极简流 -->
    <div v-else class="flex flex-col gap-3">
      <!-- 状态与耗时横幅 / 错误提示 -->
      <div
        v-if="detailTask.status === adminTaskStatuses.failed"
        class="flex items-start gap-2 rounded-xl border border-admin-danger/20 bg-admin-danger/5 p-3 text-xs leading-relaxed text-admin-danger"
        role="alert"
      >
        <WarningTriangle class="mt-0.5 size-3.5 shrink-0" />
        <div class="flex min-w-0 flex-1 flex-col gap-1">
          <div class="flex items-center justify-between">
            <span class="font-semibold">执行失败</span>
            <button
              type="button"
              class="h-5 cursor-pointer rounded border border-admin-danger/30 px-1.5 text-[11px] font-medium text-admin-danger transition-colors hover:bg-admin-danger/10"
              @click="emit('retry')"
            >
              重试
            </button>
          </div>
          <span class="text-admin-danger/90">
            {{ getErrorReason(detailTask) || '处理过程中发生未知错误' }}
          </span>
        </div>
      </div>

      <div
        v-else-if="detailTask.status === adminTaskStatuses.cancelled"
        class="flex items-start gap-2 rounded-xl border border-admin-border-soft bg-admin-surface-muted p-3 text-xs leading-relaxed text-admin-text-2-strong"
      >
        <WarningTriangle class="mt-0.5 size-3.5 shrink-0 text-admin-text-3" />
        <div class="flex min-w-0 flex-1 flex-col">
          <span class="font-semibold">任务已停止</span>
          <span class="text-admin-text-2">
            {{ getErrorReason(detailTask) || '任务已停止，未继续执行。' }}
          </span>
        </div>
      </div>

      <div
        v-else
        class="flex items-center justify-between rounded-xl border border-admin-border-soft bg-admin-surface-muted/50 px-3.5 py-2 text-xs"
      >
        <div class="flex items-center gap-1.5">
          <span class="text-admin-text-3">总耗时:</span>
          <span class="font-mono font-medium text-admin-text-1">{{ calcDuration(detailTask) }}</span>
        </div>
        <div class="flex items-center gap-1.5">
          <span class="text-admin-text-3">创建于:</span>
          <span class="font-mono font-medium text-admin-text-1">
            {{ formatDateTime(detailTask.created_at) }}
          </span>
        </div>
      </div>

      <!-- 1. 身份与归属 (Card) -->
      <section class="flex flex-col gap-1.5" aria-labelledby="task-detail-identity">
        <div class="flex items-center gap-1.5 text-xs font-semibold text-admin-text-1">
          <Fingerprint class="size-3.5 text-admin-text-2" />
          <span id="task-detail-identity">身份与归属</span>
        </div>
        <div class="flex flex-col divide-y divide-admin-border-soft/60 rounded-xl border border-admin-border-soft bg-white px-3 py-1 text-xs">
          <div class="flex items-center justify-between py-1.5">
            <span class="text-admin-text-3">用户</span>
            <div class="flex items-center gap-1">
              <span class="font-mono text-admin-text-1">
                {{ detailTask.user_display_name || detailTask.user_public_id }}
              </span>
              <button
                type="button"
                class="flex size-4 cursor-pointer items-center justify-center rounded text-admin-text-3 transition-colors hover:text-admin-text-1"
                aria-label="复制用户 Public ID"
                @click="copyToClipboard(String(detailTask.user_public_id))"
              >
                <Copy class="size-3" />
              </button>
            </div>
          </div>
          <div class="flex items-center justify-between py-1.5">
            <span class="text-admin-text-3">优先级</span>
            <span class="font-medium text-admin-text-1">{{ getTaskPriorityLabel(detailTask) }}</span>
          </div>
          <div class="flex items-center justify-between py-1.5">
            <span class="text-admin-text-3">供应商</span>
            <span class="font-mono text-admin-text-1">{{ getDetailExecutionProviderLabel(detailTask) }}</span>
          </div>
          <div class="flex items-center justify-between py-1.5">
            <span class="text-admin-text-3">模型</span>
            <span class="font-mono text-admin-text-1">{{ getDetailExecutionModelLabel(detailTask) }}</span>
          </div>
          <div class="flex items-center justify-between py-1.5">
            <span class="text-admin-text-3">创建时间</span>
            <span class="font-mono text-admin-text-1">{{ formatDateTime(detailTask.created_at) }}</span>
          </div>
          <div class="flex items-center justify-between py-1.5">
            <span class="text-admin-text-3">完成时间</span>
            <span class="font-mono text-admin-text-1">{{ formatDateTime(detailTask.completed_at) }}</span>
          </div>
        </div>
      </section>

      <!-- 2. 任务指令 (Prompt) -->
      <section class="flex flex-col gap-1.5" aria-labelledby="task-detail-prompt">
        <div class="flex items-center justify-between text-xs font-semibold text-admin-text-1">
          <div class="flex items-center gap-1.5">
            <ChatBubble class="size-3.5 text-admin-text-2" />
            <span id="task-detail-prompt">任务指令 (Prompt)</span>
          </div>
          <button
            type="button"
            class="cursor-pointer text-[11px] font-medium text-admin-text-3 transition-colors hover:text-admin-text-1 disabled:pointer-events-none disabled:opacity-50"
            :disabled="!detailTask.prompt"
            @click="copyToClipboard(detailTask.prompt || '')"
          >
            复制全文
          </button>
        </div>
        <div class="max-h-[120px] overflow-y-auto rounded-xl border border-admin-border-soft bg-admin-surface-muted/60 p-2.5">
          <div class="whitespace-pre-wrap break-words font-mono text-xs leading-relaxed text-slate-800">
            {{ detailTask.prompt || '无提示词' }}
          </div>
        </div>
      </section>

      <!-- 3. 生成结果 (Result) -->
      <section class="flex flex-col gap-1.5" aria-labelledby="task-detail-result">
        <div class="flex items-center gap-1.5 text-xs font-semibold text-admin-text-1">
          <MediaImage class="size-3.5 text-admin-text-2" />
          <span id="task-detail-result">生成结果</span>
        </div>
        <div
          v-if="resultImageUrl"
          class="group relative overflow-hidden rounded-xl border border-admin-border-soft bg-admin-surface-muted/40 p-1.5 transition-colors hover:border-admin-border-hover"
        >
          <img
            :src="resultImageUrl"
            alt="任务生成结果图片"
            class="block max-h-[240px] w-full rounded-lg object-contain"
          />
          <AdminButton
            severity="secondary"
            size="small"
            class="!absolute !right-3 !bottom-3 !opacity-0 transition-opacity group-hover:!opacity-100 group-focus-within:!opacity-100 focus-visible:!opacity-100"
            @click="openResultImage(resultImageUrl)"
          >
            在新窗口打开
          </AdminButton>
        </div>
        <div
          v-else
          class="rounded-xl border border-admin-border-soft bg-admin-surface-muted/50 px-3.5 py-2.5 text-xs text-admin-text-3"
        >
          {{
            isResultImageDeletedByUser
              ? '图片已由用户删除。'
              : hasResultImageCandidate
                ? '结果图片链接不安全，已拒绝预览。'
                : '没有可预览的图片结果。'
          }}
        </div>
      </section>

      <!-- 4. 执行轨迹 (Execution Attempts) -->
      <section class="flex flex-col gap-1.5" aria-labelledby="task-detail-execution-trace">
        <div class="flex items-center justify-between text-xs font-semibold text-admin-text-1">
          <div class="flex items-center gap-1.5">
            <Clock class="size-3.5 text-admin-text-2" />
            <span id="task-detail-execution-trace">执行轨迹</span>
          </div>
          <span class="text-[11px] font-normal text-admin-text-3">
            {{ detailTask.execution_attempts?.length || 0 }} 次尝试
          </span>
        </div>

        <div v-if="detailTask.execution_attempts?.length" class="flex flex-col gap-2">
          <article
            v-for="attempt in detailTask.execution_attempts"
            :key="attempt.attempt_no"
            class="flex flex-col gap-1.5 rounded-xl border border-admin-border-soft bg-white p-2.5 text-xs"
          >
            <div class="flex items-center justify-between gap-2">
              <div class="flex min-w-0 items-center gap-1.5">
                <span class="font-mono text-admin-text-3">#{{ attempt.attempt_no }}</span>
                <span class="font-semibold text-admin-text-1">{{ getAttemptProviderLabel(attempt) }}</span>
                <span class="text-admin-text-3">/</span>
                <span class="truncate font-mono text-[11px] text-admin-text-2">
                  {{ attempt.resolved_model || '-' }}
                </span>
              </div>
              <AdminBadge
                :value="getAttemptStatusLabel(attempt.status)"
                :severity="getAttemptStatusSeverity(attempt.status)"
                rounded
              />
            </div>
            <div class="grid grid-cols-2 gap-x-3 gap-y-1 text-[11px] text-admin-text-2 pt-1 border-t border-admin-border-soft/50">
              <div class="flex items-center justify-between">
                <span class="text-admin-text-3">阶段:</span>
                <span>{{ getAttemptStageLabel(attempt.stage) }}</span>
              </div>
              <div class="flex items-center justify-between">
                <span class="text-admin-text-3">失败分类:</span>
                <span>{{ getAttemptFailureLabel(attempt.failure_category) }}</span>
              </div>
              <div class="flex items-center justify-between">
                <span class="text-admin-text-3">Provider 耗时:</span>
                <span class="font-mono">{{ formatAttemptLatency(attempt.provider_latency_ms) }}</span>
              </div>
              <div class="flex items-center justify-between">
                <span class="text-admin-text-3">总耗时:</span>
                <span class="font-mono">{{ formatAttemptLatency(attempt.total_latency_ms) }}</span>
              </div>
            </div>
          </article>
        </div>
        <div
          v-else
          class="rounded-xl border border-admin-border-soft bg-admin-surface-muted/50 py-3 text-center text-xs text-admin-text-3"
        >
          暂无执行记录
        </div>
      </section>

      <!-- 5. 计费快照 (Billing Snapshot) -->
      <section
        v-if="detailTask.billing"
        class="flex flex-col gap-1.5"
        aria-labelledby="task-detail-billing"
      >
        <div class="flex items-center justify-between text-xs font-semibold text-admin-text-1">
          <div class="flex items-center gap-1.5">
            <LightBulb class="size-3.5 text-admin-text-2" />
            <span id="task-detail-billing">计费快照</span>
          </div>
          <span
            v-if="!detailTask.billing.feature_exists"
            class="text-[11px] font-normal text-admin-text-3"
          >
            功能已删除
          </span>
        </div>
        <div class="flex flex-col divide-y divide-admin-border-soft/60 rounded-xl border border-admin-border-soft bg-white px-3 py-1 text-xs">
          <div class="flex items-center justify-between py-1.5">
            <span class="text-admin-text-3">功能</span>
            <span class="font-medium text-admin-text-1">{{ detailTask.billing.display_name }}</span>
          </div>
          <div class="flex items-center justify-between py-1.5">
            <span class="text-admin-text-3">SKU</span>
            <span class="font-mono text-admin-text-1">{{ detailTask.billing.sku }}</span>
          </div>
          <div class="flex items-center justify-between py-1.5">
            <span class="text-admin-text-3">单价</span>
            <span class="font-mono text-admin-text-1">{{ formatIdeaAmount(detailTask.billing.unit_credits) }}</span>
          </div>
          <div class="flex items-center justify-between py-1.5">
            <span class="text-admin-text-3">调用数量</span>
            <span class="font-mono text-admin-text-1">{{ detailTask.billing.quantity }}</span>
          </div>
          <div class="flex items-center justify-between py-1.5">
            <span class="text-admin-text-3">总消耗</span>
            <span class="font-mono font-semibold text-admin-text-1">
              {{ formatIdeaAmount(detailTask.billing.total_credits) }}
            </span>
          </div>
          <div class="flex items-center justify-between py-1.5">
            <span class="text-admin-text-3">结算状态</span>
            <span class="font-medium text-admin-text-1">
              {{ getBillingSettlementLabel(detailTask.billing.settlement_state) }}
            </span>
          </div>
          <div class="flex items-center justify-between py-1.5">
            <span class="text-admin-text-3">报价时间</span>
            <span class="font-mono text-admin-text-1">{{ formatDateTime(detailTask.billing.quoted_at) }}</span>
          </div>
          <div class="flex items-center justify-between py-1.5">
            <span class="text-admin-text-3">结算时间</span>
            <span class="font-mono text-admin-text-1">{{ formatDateTime(detailTask.billing.settled_at) }}</span>
          </div>
        </div>
      </section>

      <!-- 6. API 响应摘要 & 调试详情 -->
      <section class="flex flex-col gap-1.5" aria-labelledby="task-detail-api-result">
        <div class="flex items-center justify-between text-xs font-semibold text-admin-text-1">
          <div class="flex items-center gap-1.5">
            <CodeBrackets class="size-3.5 text-admin-text-2" />
            <span id="task-detail-api-result">API 响应摘要</span>
          </div>
          <button
            type="button"
            class="cursor-pointer text-[11px] font-medium text-admin-text-3 transition-colors hover:text-admin-text-1 disabled:pointer-events-none disabled:opacity-50"
            :disabled="!hasResultPayload"
            @click="copyToClipboard(resultSummaryText)"
          >
            复制 JSON
          </button>
        </div>
        <div class="max-h-[140px] overflow-x-auto rounded-xl border border-admin-border-soft bg-admin-surface-muted/60 p-2.5 font-mono text-xs">
          <pre class="m-0 leading-relaxed text-slate-800">{{
            resultSummaryText || '暂无 API 响应结果'
          }}</pre>
        </div>

        <!-- 原始调试详情 (脱敏) -->
        <div
          v-if="resultDebugText"
          class="overflow-hidden rounded-xl border border-admin-border-soft bg-admin-surface-muted/40"
        >
          <button
            type="button"
            class="flex w-full cursor-pointer items-center justify-between px-3 py-2 text-left text-xs font-medium text-admin-text-2 transition-colors hover:bg-admin-surface-muted"
            :aria-expanded="showDebugResult"
            aria-controls="task-result-debug-details"
            @click="showDebugResult = !showDebugResult"
          >
            <div class="flex items-center gap-1.5">
              <Terminal class="size-3.5 text-admin-text-3" />
              <span>{{ `原始调试详情 (脱敏) · ${showDebugResult ? '隐藏' : '展开'}` }}</span>
            </div>
            <NavArrowDown
              class="size-3 text-admin-text-3 transition-transform"
              :class="showDebugResult ? 'rotate-180' : ''"
            />
          </button>
          <div
            v-show="showDebugResult"
            id="task-result-debug-details"
            class="overflow-x-auto border-t border-admin-border-soft bg-white p-2.5 font-mono text-xs text-admin-text-3"
          >
            <pre class="m-0 leading-relaxed">{{ resultDebugText }}</pre>
          </div>
        </div>
      </section>
    </div>
  </AdminDialogShell>
</template>

<script setup lang="ts">
import { watch } from 'vue'
import {
  ChatBubble,
  CheckCircle,
  Clock,
  CodeBrackets,
  Copy,
  Fingerprint,
  LightBulb,
  MediaImage,
  NavArrowDown,
  Terminal,
  WarningTriangle,
  XmarkCircle,
} from '@iconoir/vue'
import { DialogTitle } from 'reka-ui'
import AdminBadge from '../../shared/components/AdminBadge.vue'
import AdminButton from '../../shared/components/AdminButton.vue'
import AdminDialogShell from '../../shared/components/AdminDialogShell.vue'
import { formatIdeaAmount } from '../../../../shared/i18n/userIdeaCurrency'
import { adminActionButtonClass } from '../../shared/lib/uiClasses'
import { getTaskPriorityLabel } from './columns'
import { adminTaskStatuses, getTaskStatusDefinition } from '../domain/status'
import type { AdminMonitorDetailModel, AdminMonitorExecutionAttempt } from '../infrastructure/resource'
import { useAdminTaskResultFeedback } from '../application/useAdminMonitorFeedback'
import { useTaskResultDetail } from '../application/useTaskResultDetail'

const props = withDefaults(
  defineProps<{
    visible: boolean
    task: AdminMonitorDetailModel | null
    taskId?: string
    loading?: boolean
    errorMessage?: string
  }>(),
  {
    taskId: '',
    loading: false,
    errorMessage: '',
  },
)

const emit = defineEmits<{
  'update:visible': [visible: boolean]
  retry: []
}>()

watch(
  () => props.visible,
  (visible) => {
    if (!visible) {
      showDebugResult.value = false
    }
  },
)

const feedback = useAdminTaskResultFeedback()
const {
  detailTask,
  showDebugResult,
  resultImageUrl,
  resultSummaryText,
  resultDebugText,
  hasResultImageCandidate,
  isResultImageDeletedByUser,
  hasResultPayload,
  openResultImage,
  copyToClipboard,
  formatDateTime,
  getErrorReason,
  calcDuration,
} = useTaskResultDetail(() => props.task, { feedback })

function handleVisibleUpdate(visible: boolean) {
  emit('update:visible', visible)
}

function getDetailExecutionProviderLabel(task: AdminMonitorDetailModel) {
  return task.latest_execution?.provider || '-'
}

function getDetailExecutionModelLabel(task: AdminMonitorDetailModel) {
  const execution = task.latest_execution
  if (!execution) return '-'
  return execution.resolved_model || '-'
}

function getAttemptProviderLabel(attempt: AdminMonitorExecutionAttempt) {
  return attempt.provider || `模型 ${attempt.resolved_model || '未知'}`
}

function getAttemptStatusLabel(status: string) {
  return (
    {
      running: '执行中',
      completed: '已完成',
      failed: '失败',
      requeued: '已重试',
      cancelled: '已取消',
    }[status] || status
  )
}

function getAttemptStatusSeverity(status: string) {
  const severityByStatus: Record<string, 'success' | 'danger' | 'warn' | 'secondary' | 'info'> = {
    running: 'info',
    completed: 'success',
    failed: 'danger',
    requeued: 'warn',
    cancelled: 'secondary',
  }
  return severityByStatus[status] || 'secondary'
}

function getAttemptStageLabel(stage: string) {
  return (
    {
      queued: '排队中',
      assigned: '已分配',
      provider: 'Provider',
      finalizing: '结果落盘',
      completed: '已完成',
      failed: '失败',
      cancelled: '已取消',
    }[stage] || stage
  )
}

function getAttemptFailureLabel(category: string) {
  if (!category) return '-'
  return (
    {
      input_invalid: '输入无效',
      auth: '鉴权',
      quota: '配额',
      timeout: '超时',
      provider_error: 'Provider 错误',
      storage_error: '存储错误',
      finalize_error: '结果处理错误',
      cancelled: '已取消',
      unknown: '未知',
    }[category] || category
  )
}

function formatAttemptLatency(milliseconds: number) {
  if (milliseconds <= 0) return '-'
  if (milliseconds < 1000) return `${milliseconds} ms`
  return `${(milliseconds / 1000).toFixed(milliseconds < 10000 ? 1 : 0)} s`
}

function getBillingSettlementLabel(state: string) {
  return (
    {
      held: '待结算',
      free: '免费',
      captured: '已扣费',
      released: '已释放',
    }[state] || state
  )
}
</script>
