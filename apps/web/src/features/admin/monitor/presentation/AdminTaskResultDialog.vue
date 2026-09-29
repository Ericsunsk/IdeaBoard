<template>
  <AdminDialogShell
    :visible="visible"
    size="slim"
    content-layout="scrollable"
    :aria-label="MONITOR_MESSAGES.detailAria"
    :description="MONITOR_MESSAGES.detailDescription"
    @update:visible="handleVisibleUpdate"
  >
    <template #header>
      <div class="flex w-full items-center justify-between gap-3 pr-6">
        <div class="flex min-w-0 items-center gap-2.5">
          <span
            class="inline-flex size-7 shrink-0 items-center justify-center rounded-lg border border-admin-border-soft bg-admin-surface-muted"
            :style="{ color: getTaskStatusDefinition(detailTask?.status || '').color }"
          >
            <CheckCircle
              v-if="detailTask?.status === adminTaskStatuses.completed"
              class="size-3.5"
            />
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
              {{ MONITOR_MESSAGES.detailAria }}
            </DialogTitle>
            <div class="flex items-center gap-1.5 font-mono text-xs text-admin-text-3">
              <span class="truncate">ID: {{ detailTask?.task_id || taskId || '-' }}</span>
              <button
                v-if="detailTask?.task_id || taskId"
                type="button"
                class="flex size-5 cursor-pointer items-center justify-center rounded text-admin-text-3 transition-colors hover:bg-admin-surface-muted hover:text-admin-text-1"
                :aria-label="MONITOR_MESSAGES.copyTaskIdAria"
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
      {{ MONITOR_MESSAGES.detailLoading }}
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
        {{ MONITOR_MESSAGES.retry }}
      </AdminButton>
    </div>

    <div
      v-else-if="!detailTask"
      class="flex min-h-40 items-center justify-center rounded-xl border border-admin-border-soft bg-admin-surface-muted/60 text-xs text-admin-text-3"
    >
      {{ MONITOR_MESSAGES.detailUnavailable }}
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
            <span class="font-semibold">{{ MONITOR_MESSAGES.detailFailedTitle }}</span>
            <button
              type="button"
              class="h-5 cursor-pointer rounded border border-admin-danger/30 px-1.5 text-admin-caption font-medium text-admin-danger transition-colors hover:bg-admin-danger/10"
              @click="emit('retry')"
            >
              {{ MONITOR_MESSAGES.retry }}
            </button>
          </div>
          <span class="text-admin-danger/90">
            {{ getErrorReason(detailTask) || MONITOR_MESSAGES.detailUnknownError }}
          </span>
        </div>
      </div>

      <div
        v-else-if="detailTask.status === adminTaskStatuses.cancelled"
        class="flex items-start gap-2 rounded-xl border border-admin-border-soft bg-admin-surface-muted p-3 text-xs leading-relaxed text-admin-text-2-strong"
      >
        <WarningTriangle class="mt-0.5 size-3.5 shrink-0 text-admin-text-3" />
        <div class="flex min-w-0 flex-1 flex-col">
          <span class="font-semibold">{{ MONITOR_MESSAGES.detailStoppedTitle }}</span>
          <span class="text-admin-text-2">
            {{ getErrorReason(detailTask) || MONITOR_MESSAGES.detailStoppedFallback }}
          </span>
        </div>
      </div>

      <div
        v-else
        class="flex items-center justify-between rounded-xl border border-admin-border-soft bg-admin-surface-muted/50 px-3.5 py-2 text-xs"
      >
        <div class="flex items-center gap-1.5">
          <span class="text-admin-text-3">{{ MONITOR_MESSAGES.detailDurationLabel }}</span>
          <span class="font-mono font-medium text-admin-text-1">{{
            calcDuration(detailTask)
          }}</span>
        </div>
        <div class="flex items-center gap-1.5">
          <span class="text-admin-text-3">{{ MONITOR_MESSAGES.detailCreatedLabel }}</span>
          <span class="font-mono font-medium text-admin-text-1">
            {{ formatDateTime(detailTask.created_at) }}
          </span>
        </div>
      </div>

      <!-- 1. 身份与归属 (Card) -->
      <section class="flex flex-col gap-1.5" aria-labelledby="task-detail-identity">
        <div class="flex items-center gap-1.5 text-xs font-semibold text-admin-text-1">
          <Fingerprint class="size-3.5 text-admin-text-2" />
          <span id="task-detail-identity">{{ MONITOR_MESSAGES.identityTitle }}</span>
        </div>
        <div
          class="flex flex-col divide-y divide-admin-border-soft/60 rounded-xl border border-admin-border-soft bg-white px-3 py-1 text-xs"
        >
          <div class="flex items-center justify-between py-1.5">
            <span class="text-admin-text-3">{{ MONITOR_MESSAGES.userLabel }}</span>
            <div class="flex items-center gap-1">
              <span class="font-mono text-admin-text-1">
                {{ detailTask.user_display_name || detailTask.user_public_id }}
              </span>
              <button
                type="button"
                class="flex size-4 cursor-pointer items-center justify-center rounded text-admin-text-3 transition-colors hover:text-admin-text-1"
                :aria-label="MONITOR_MESSAGES.copyUserIdAria"
                @click="copyToClipboard(String(detailTask.user_public_id))"
              >
                <Copy class="size-3" />
              </button>
            </div>
          </div>
          <div class="flex items-center justify-between py-1.5">
            <span class="text-admin-text-3">{{ MONITOR_MESSAGES.priorityLabel }}</span>
            <span class="font-medium text-admin-text-1">{{
              getTaskPriorityLabel(detailTask)
            }}</span>
          </div>
          <div class="flex items-center justify-between py-1.5">
            <span class="text-admin-text-3">{{ MONITOR_MESSAGES.providerLabel }}</span>
            <span class="font-mono text-admin-text-1">{{
              getDetailExecutionProviderLabel(detailTask)
            }}</span>
          </div>
          <div class="flex items-center justify-between py-1.5">
            <span class="text-admin-text-3">{{ MONITOR_MESSAGES.modelLabel }}</span>
            <span class="font-mono text-admin-text-1">{{
              getDetailExecutionModelLabel(detailTask)
            }}</span>
          </div>
          <div class="flex items-center justify-between py-1.5">
            <span class="text-admin-text-3">{{ MONITOR_MESSAGES.createdAtLabel }}</span>
            <span class="font-mono text-admin-text-1">{{
              formatDateTime(detailTask.created_at)
            }}</span>
          </div>
          <div class="flex items-center justify-between py-1.5">
            <span class="text-admin-text-3">{{ MONITOR_MESSAGES.completedAtLabel }}</span>
            <span class="font-mono text-admin-text-1">{{
              formatDateTime(detailTask.completed_at)
            }}</span>
          </div>
        </div>
      </section>

      <!-- 2. 任务指令 (Prompt) -->
      <section class="flex flex-col gap-1.5" aria-labelledby="task-detail-prompt">
        <div class="flex items-center justify-between text-xs font-semibold text-admin-text-1">
          <div class="flex items-center gap-1.5">
            <ChatBubble class="size-3.5 text-admin-text-2" />
            <span id="task-detail-prompt">{{ MONITOR_MESSAGES.promptTitle }}</span>
          </div>
          <button
            type="button"
            class="cursor-pointer text-admin-caption font-medium text-admin-text-3 transition-colors hover:text-admin-text-1 disabled:pointer-events-none disabled:opacity-50"
            :disabled="!detailTask.prompt"
            @click="copyToClipboard(detailTask.prompt || '')"
          >
            {{ MONITOR_MESSAGES.copyPrompt }}
          </button>
        </div>
        <div
          class="max-h-[120px] overflow-y-auto rounded-xl border border-admin-border-soft bg-admin-surface-muted/60 p-2.5"
        >
          <div
            class="whitespace-pre-wrap break-words font-mono text-xs leading-relaxed text-slate-800"
          >
            {{ detailTask.prompt || MONITOR_MESSAGES.promptEmpty }}
          </div>
        </div>
      </section>

      <!-- 2. 路由快照 (Routing Snapshot, #852) -->
      <section class="flex flex-col gap-1.5" aria-labelledby="task-detail-routing">
        <div class="flex items-center gap-1.5 text-xs font-semibold text-admin-text-1">
          <FastArrowRight class="size-3.5 text-admin-text-2" />
          <span id="task-detail-routing">{{ MONITOR_MESSAGES.routingTitle }}</span>
        </div>
        <div
          v-if="detailTask.routing"
          class="flex flex-col divide-y divide-admin-border-soft/60 rounded-xl border border-admin-border-soft bg-white px-3 py-1 text-xs"
        >
          <div class="flex items-center justify-between py-1.5">
            <span class="text-admin-text-3">{{ MONITOR_MESSAGES.routingServiceLabel }}</span>
            <span class="font-mono text-admin-text-1">{{ detailTask.routing.service }}</span>
          </div>
          <div class="flex items-center justify-between py-1.5">
            <span class="text-admin-text-3">{{ MONITOR_MESSAGES.routingResolutionLabel }}</span>
            <span class="font-mono text-admin-text-1">{{
              detailTask.routing.requested_resolution || '-'
            }}</span>
          </div>
          <div class="flex items-center justify-between py-1.5">
            <span class="text-admin-text-3">{{ MONITOR_MESSAGES.routingPoolVersionLabel }}</span>
            <span class="font-mono text-admin-text-1">v{{ detailTask.routing.pool_version }}</span>
          </div>
          <div class="flex items-center justify-between py-1.5">
            <span class="text-admin-text-3">{{ MONITOR_MESSAGES.routingSelectedLabel }}</span>
            <span class="font-mono text-admin-text-1">{{
              detailTask.routing.selected_model || '-'
            }}</span>
          </div>
          <div class="flex items-center justify-between py-1.5">
            <span class="text-admin-text-3">{{ MONITOR_MESSAGES.routingFinalLabel }}</span>
            <span class="font-mono font-medium text-admin-text-1">{{
              getDetailFinalModelLabel(detailTask)
            }}</span>
          </div>
          <div
            v-if="detailTask.routing.retry_candidate"
            class="flex items-center justify-between py-1.5"
          >
            <span class="text-admin-text-3">{{ MONITOR_MESSAGES.routingRetryCandidateLabel }}</span>
            <span class="font-mono text-admin-text-1">{{
              detailTask.routing.retry_candidate
            }}</span>
          </div>
          <div class="flex items-center justify-between gap-3 py-1.5">
            <span class="shrink-0 text-admin-text-3">{{
              MONITOR_MESSAGES.routingEligibleLabel
            }}</span>
            <span class="truncate font-mono text-admin-text-1" :title="eligibleModelsText">
              {{ eligibleModelsText }}
            </span>
          </div>
        </div>
        <div
          v-else
          class="rounded-xl border border-admin-border-soft bg-admin-surface-muted/50 px-3.5 py-2.5 text-xs text-admin-text-3"
        >
          {{ MONITOR_MESSAGES.routingMissing }}
        </div>
      </section>

      <!-- 3. 生成结果 (Result) -->
      <section class="flex flex-col gap-1.5" aria-labelledby="task-detail-result">
        <div class="flex items-center gap-1.5 text-xs font-semibold text-admin-text-1">
          <MediaImage class="size-3.5 text-admin-text-2" />
          <span id="task-detail-result">{{ MONITOR_MESSAGES.resultTitle }}</span>
        </div>
        <div
          v-if="resultImageUrl"
          class="group relative overflow-hidden rounded-xl border border-admin-border-soft bg-admin-surface-muted/40 p-1.5 transition-colors hover:border-admin-border-hover"
        >
          <img
            :src="resultImageUrl"
            :alt="MONITOR_MESSAGES.resultImageAlt"
            class="block max-h-[240px] w-full rounded-lg object-contain"
          />
          <AdminButton
            severity="secondary"
            size="small"
            class="!absolute !right-3 !bottom-3 !opacity-0 transition-opacity group-hover:!opacity-100 group-focus-within:!opacity-100 focus-visible:!opacity-100"
            @click="openResultImage(resultImageUrl)"
          >
            {{ MONITOR_MESSAGES.openResultImage }}
          </AdminButton>
        </div>
        <div
          v-else
          class="rounded-xl border border-admin-border-soft bg-admin-surface-muted/50 px-3.5 py-2.5 text-xs text-admin-text-3"
        >
          {{
            isResultImageDeletedByUser
              ? MONITOR_MESSAGES.resultDeleted
              : hasResultImageCandidate
                ? MONITOR_MESSAGES.resultUnsafe
                : MONITOR_MESSAGES.resultEmpty
          }}
        </div>
      </section>

      <!-- 4. 执行轨迹 (Execution Attempts) -->
      <section class="flex flex-col gap-1.5" aria-labelledby="task-detail-execution-trace">
        <div class="flex items-center justify-between text-xs font-semibold text-admin-text-1">
          <div class="flex items-center gap-1.5">
            <Clock class="size-3.5 text-admin-text-2" />
            <span id="task-detail-execution-trace">{{ MONITOR_MESSAGES.attemptsTitle }}</span>
          </div>
          <span class="text-admin-caption font-normal text-admin-text-3">
            {{ MONITOR_MESSAGES.attemptsCount(detailTask.execution_attempts?.length || 0) }}
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
                <span class="font-semibold text-admin-text-1">{{
                  getAttemptProviderLabel(attempt)
                }}</span>
                <span class="text-admin-text-3">/</span>
                <span class="truncate font-mono text-admin-caption text-admin-text-2">
                  {{ attempt.resolved_model || '-' }}
                </span>
              </div>
              <AdminBadge
                :value="getAttemptStatusLabel(attempt.status)"
                :severity="getAttemptStatusSeverity(attempt.status)"
                rounded
              />
            </div>
            <div
              class="grid grid-cols-2 gap-x-3 gap-y-1 text-admin-caption text-admin-text-2 pt-1 border-t border-admin-border-soft/50"
            >
              <div class="flex items-center justify-between">
                <span class="text-admin-text-3">{{ MONITOR_MESSAGES.attemptStageLabel }}</span>
                <span>{{ getAttemptStageLabel(attempt.stage) }}</span>
              </div>
              <div class="flex items-center justify-between">
                <span class="text-admin-text-3">{{ MONITOR_MESSAGES.attemptFailureLabel }}</span>
                <span>{{ getAttemptFailureLabel(attempt.failure_category) }}</span>
              </div>
              <div class="flex items-center justify-between">
                <span class="text-admin-text-3">{{ MONITOR_MESSAGES.attemptLayerLabel }}</span>
                <span>{{ getAttemptRetryLayerLabel(attempt.retry_layer) }}</span>
              </div>
              <div
                v-if="(attempt.gateway_retry_count ?? 0) > 0"
                class="flex items-center justify-between"
              >
                <span class="text-admin-text-3">{{
                  MONITOR_MESSAGES.attemptGatewayRetriesLabel
                }}</span>
                <span class="font-mono">{{ attempt.gateway_retry_count }}</span>
              </div>
              <div class="flex items-center justify-between">
                <span class="text-admin-text-3">{{
                  MONITOR_MESSAGES.attemptProviderLatencyLabel
                }}</span>
                <span class="font-mono">{{
                  formatAttemptLatency(attempt.provider_latency_ms)
                }}</span>
              </div>
              <div class="flex items-center justify-between">
                <span class="text-admin-text-3">{{
                  MONITOR_MESSAGES.attemptTotalLatencyLabel
                }}</span>
                <span class="font-mono">{{ formatAttemptLatency(attempt.total_latency_ms) }}</span>
              </div>
            </div>
          </article>
        </div>
        <div
          v-else
          class="rounded-xl border border-admin-border-soft bg-admin-surface-muted/50 py-3 text-center text-xs text-admin-text-3"
        >
          {{ MONITOR_MESSAGES.attemptsEmpty }}
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
            <span id="task-detail-billing">{{ MONITOR_MESSAGES.billingTitle }}</span>
          </div>
          <span
            v-if="detailTask.billing.feature_exists === false"
            class="text-admin-caption font-normal text-admin-text-3"
          >
            {{ MONITOR_MESSAGES.billingFeatureGone }}
          </span>
        </div>
        <div
          class="flex flex-col divide-y divide-admin-border-soft/60 rounded-xl border border-admin-border-soft bg-white px-3 py-1 text-xs"
        >
          <div class="flex items-center justify-between py-1.5">
            <span class="text-admin-text-3">{{ MONITOR_MESSAGES.billingFeatureLabel }}</span>
            <span class="font-medium text-admin-text-1">{{ detailTask.billing.display_name }}</span>
          </div>
          <div class="flex items-center justify-between py-1.5">
            <span class="text-admin-text-3">SKU</span>
            <span class="font-mono text-admin-text-1">{{ detailTask.billing.sku }}</span>
          </div>
          <div class="flex items-center justify-between py-1.5">
            <span class="text-admin-text-3">{{ MONITOR_MESSAGES.billingUnitLabel }}</span>
            <span class="font-mono text-admin-text-1">{{
              formatIdeaAmount(detailTask.billing.unit_credits)
            }}</span>
          </div>
          <div class="flex items-center justify-between py-1.5">
            <span class="text-admin-text-3">{{ MONITOR_MESSAGES.billingQuantityLabel }}</span>
            <span class="font-mono text-admin-text-1">{{ detailTask.billing.quantity }}</span>
          </div>
          <div class="flex items-center justify-between py-1.5">
            <span class="text-admin-text-3">{{ MONITOR_MESSAGES.billingTotalLabel }}</span>
            <span class="font-mono font-semibold text-admin-text-1">
              {{ formatIdeaAmount(detailTask.billing.total_credits) }}
            </span>
          </div>
          <div class="flex items-center justify-between py-1.5">
            <span class="text-admin-text-3">{{ MONITOR_MESSAGES.billingSettlementLabel }}</span>
            <span class="font-medium text-admin-text-1">
              {{ getBillingSettlementLabel(detailTask.billing.settlement_state) }}
            </span>
          </div>
          <div class="flex items-center justify-between py-1.5">
            <span class="text-admin-text-3">{{ MONITOR_MESSAGES.billingQuotedLabel }}</span>
            <span class="font-mono text-admin-text-1">{{
              formatDateTime(detailTask.billing.quoted_at)
            }}</span>
          </div>
          <div class="flex items-center justify-between py-1.5">
            <span class="text-admin-text-3">{{ MONITOR_MESSAGES.billingSettledLabel }}</span>
            <span class="font-mono text-admin-text-1">{{
              formatDateTime(detailTask.billing.settled_at)
            }}</span>
          </div>
        </div>
      </section>

      <!-- 6. API 响应摘要 & 调试详情 -->
      <section class="flex flex-col gap-1.5" aria-labelledby="task-detail-api-result">
        <div class="flex items-center justify-between text-xs font-semibold text-admin-text-1">
          <div class="flex items-center gap-1.5">
            <CodeBrackets class="size-3.5 text-admin-text-2" />
            <span id="task-detail-api-result">{{ MONITOR_MESSAGES.apiResultTitle }}</span>
          </div>
          <button
            type="button"
            class="cursor-pointer text-admin-caption font-medium text-admin-text-3 transition-colors hover:text-admin-text-1 disabled:pointer-events-none disabled:opacity-50"
            :disabled="!hasResultPayload"
            @click="copyToClipboard(resultSummaryText)"
          >
            {{ MONITOR_MESSAGES.copyJson }}
          </button>
        </div>
        <div
          class="max-h-[140px] overflow-x-auto rounded-xl border border-admin-border-soft bg-admin-surface-muted/60 p-2.5 font-mono text-xs"
        >
          <pre class="m-0 leading-relaxed text-slate-800">{{
            resultSummaryText || MONITOR_MESSAGES.apiResultEmpty
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
              <span>{{ MONITOR_MESSAGES.debugToggle(showDebugResult) }}</span>
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
import { computed, watch } from 'vue'
import {
  ChatBubble,
  CheckCircle,
  Clock,
  CodeBrackets,
  Copy,
  FastArrowRight,
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
import type { AdminMonitorDetailModel } from '../infrastructure/resource'
import { useAdminTaskResultFeedback } from '../application/useAdminMonitorFeedback'
import { MONITOR_MESSAGES } from './pages/messages'
import { useTaskResultDetail } from '../application/useTaskResultDetail'
import {
  formatAttemptLatency,
  getAttemptFailureLabel,
  getAttemptProviderLabel,
  getAttemptRetryLayerLabel,
  getAttemptStageLabel,
  getAttemptStatusLabel,
  getAttemptStatusSeverity,
  getBillingSettlementLabel,
  getDetailExecutionModelLabel,
  getDetailExecutionProviderLabel,
  getDetailFinalModelLabel,
} from './taskResultLabels'

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

const eligibleModelsText = computed(() => {
  const models = props.task?.routing?.eligible_models ?? []
  return models.length > 0 ? models.join(', ') : '-'
})
</script>
