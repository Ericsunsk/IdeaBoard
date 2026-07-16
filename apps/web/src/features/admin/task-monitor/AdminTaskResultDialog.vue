<template>
  <Dialog
    :visible="visible"
    modal
    :style="{ width: '680px', maxWidth: '95vw' }"
    :pt="taskDetailDialogPt"
    :draggable="false"
    aria-label="任务详情控制台"
    @update:visible="handleVisibleUpdate"
  >
    <template #header>
      <div class="flex w-full items-center justify-between gap-3.5">
        <div class="flex min-w-0 items-center gap-3">
          <span
            class="inline-flex h-[38px] w-[38px] shrink-0 items-center justify-center rounded-[14px] border border-admin-border-soft bg-admin-surface-muted"
            :style="{ color: getTaskStatusDefinition(detailTask?.status || '').color }"
          >
            <CheckCircle v-if="detailTask?.status === adminTaskStatuses.completed" />
            <XmarkCircle
              v-else-if="
                detailTask?.status === adminTaskStatuses.failed ||
                detailTask?.status === adminTaskStatuses.cancelled
              "
            />
            <Clock v-else />
          </span>
          <div class="flex min-w-0 flex-col">
            <span class="text-admin-title-sm leading-tight font-bold text-admin-title-color">
              任务详情控制台
            </span>
            <span class="mt-1 truncate font-mono text-admin-caption text-admin-subtitle-color">
              ID: {{ detailTask?.task_id || '-' }}
            </span>
          </div>
        </div>
        <div class="flex shrink-0 items-center gap-2">
          <Tag
            v-if="detailTask"
            :value="getTaskStatusDefinition(detailTask.status).text"
            :severity="getTaskStatusDefinition(detailTask.status).severity"
            rounded
            class="max-md:hidden"
          />
          <Button
            text
            rounded
            size="small"
            :class="adminTextButtonClass"
            aria-label="复制任务 ID"
            :disabled="!detailTask?.task_id"
            @click="copyToClipboard(detailTask?.task_id || '')"
          >
            <template #icon><Copy /></template>
          </Button>
        </div>
      </div>
    </template>

    <div v-if="detailTask" class="flex flex-col gap-[18px]">
      <div :class="getTaskStatusBannerClass(detailTask.status)">
        <div class="flex flex-col gap-1.5">
          <span class="text-admin-caption font-bold text-admin-text-3">当前状态</span>
          <span class="flex items-center gap-[9px]">
            <span :class="getTaskStatusDotClass(detailTask.status)" />
            <span class="text-admin-heading leading-[1.15] font-extrabold text-admin-text-1">
              {{ getTaskStatusDefinition(detailTask.status).text }}
            </span>
          </span>
        </div>
        <div
          v-if="detailTask.status === adminTaskStatuses.failed"
          class="flex items-start gap-2.5 rounded-xl border border-admin-danger/20 bg-admin-danger/10 px-3 py-2.5 text-admin-body leading-[1.55] text-admin-danger"
        >
          <WarningTriangle class="mt-0.5 shrink-0" />
          <span>{{ getErrorReason(detailTask) || '处理过程中发生未知错误' }}</span>
        </div>
        <div
          v-else-if="detailTask.status === adminTaskStatuses.cancelled"
          class="flex items-start gap-2.5 rounded-xl border border-admin-border bg-white/70 px-3 py-2.5 text-admin-body leading-[1.55] text-admin-text-2-strong"
        >
          <WarningTriangle class="mt-0.5 shrink-0 text-admin-text-3" />
          <span>{{ getErrorReason(detailTask) || '任务已停止，未继续执行。' }}</span>
        </div>
        <div
          v-else
          class="grid grid-cols-1 gap-2.5 border-t border-admin-divider pt-3 md:grid-cols-2"
        >
          <DetailMetric label="总耗时" :value="calcDuration(detailTask)" />
          <DetailMetric
            label="创建于"
            :value="formatDateTime(detailTask.created_at).split('-')[1] || '-'"
          />
        </div>
      </div>

      <section class="flex flex-col gap-2.5" aria-labelledby="task-detail-identity">
        <div
          class="flex min-w-0 items-center gap-2 text-admin-body font-bold text-admin-title-color"
        >
          <Fingerprint class="h-[17px] w-[17px] shrink-0 text-admin-subtitle-color" />
          <span id="task-detail-identity">身份与归属</span>
        </div>
        <div class="grid grid-cols-1 gap-2.5 md:grid-cols-2">
          <div :class="detailMetricClass">
            <span class="text-admin-caption font-bold text-admin-text-3">用户 ID</span>
            <div class="flex min-w-0 items-center justify-between gap-2">
              <span class="min-w-0 truncate text-admin-body font-semibold text-admin-text-1">{{
                detailTask.user_id
              }}</span>
              <Button
                text
                rounded
                size="small"
                :class="adminTextButtonClass"
                aria-label="复制用户 ID"
                @click="copyToClipboard(String(detailTask.user_id))"
              >
                <template #icon><Copy /></template>
              </Button>
            </div>
          </div>
          <DetailMetric label="优先级" :value="getTaskPriorityLabel(detailTask)" />
          <DetailMetric label="账号 ID" :value="String(detailTask.account_id || 'N/A')" />
          <DetailMetric
            label="Endpoint"
            :value="getTaskEndpointLabel(detailTask) || 'N/A'"
            :title="detailTask.provider_endpoint_base_url || getTaskEndpointLabel(detailTask)"
          />
          <DetailMetric label="Credential" :value="getTaskCredentialLabel(detailTask) || 'N/A'" />
          <DetailMetric
            label="模型 / API Type"
            :value="`${getTaskModelLabel(detailTask) || 'N/A'} · ${detailTask.api_type || 'N/A'}`"
          />
          <DetailMetric label="创建时间" :value="formatDateTime(detailTask.created_at)" />
          <DetailMetric label="完成时间" :value="formatDateTime(detailTask.completed_at)" />
        </div>
      </section>

      <section
        v-if="detailTask.billing"
        class="flex flex-col gap-2.5"
        aria-labelledby="task-detail-billing"
      >
        <div
          class="flex min-w-0 items-center gap-2 text-admin-body font-bold text-admin-title-color"
        >
          <LightBulb class="h-[17px] w-[17px] shrink-0 text-admin-subtitle-color" />
          <span id="task-detail-billing">计费快照</span>
          <Tag
            v-if="!detailTask.billing.feature_exists"
            value="功能已删除"
            severity="secondary"
            rounded
            class="ml-auto"
          />
        </div>
        <div class="grid grid-cols-1 gap-2.5 md:grid-cols-2">
          <DetailMetric label="功能" :value="detailTask.billing.display_name" />
          <DetailMetric label="SKU" :value="detailTask.billing.sku" />
          <DetailMetric label="单价" :value="formatIdeaAmount(detailTask.billing.unit_credits)" />
          <DetailMetric label="调用数量" :value="String(detailTask.billing.quantity)" />
          <DetailMetric label="总消耗" :value="formatIdeaAmount(detailTask.billing.total_credits)" />
          <DetailMetric label="报价时间" :value="formatDateTime(detailTask.billing.quoted_at)" />
        </div>
      </section>

      <section class="flex flex-col gap-2.5" aria-labelledby="task-detail-prompt">
        <div
          class="flex min-w-0 items-center gap-2 text-admin-body font-bold text-admin-title-color"
        >
          <ChatBubble class="h-[17px] w-[17px] shrink-0 text-admin-subtitle-color" />
          <span id="task-detail-prompt">任务指令 (Prompt)</span>
          <Button
            text
            size="small"
            label="复制全文"
            :class="[adminTextButtonClass, 'ml-auto !text-admin-caption !text-admin-text-3']"
            :disabled="!detailTask.prompt"
            @click="copyToClipboard(detailTask.prompt || '')"
          />
        </div>
        <div
          class="max-h-[210px] overflow-y-auto rounded-[14px] border border-admin-border-soft bg-white/80 px-4 py-3.5"
        >
          <div
            class="whitespace-pre-wrap break-words text-admin-body leading-[1.7] text-admin-text-2-strong"
          >
            {{ detailTask.prompt || '无提示词' }}
          </div>
        </div>
      </section>

      <section class="flex flex-col gap-2.5" aria-labelledby="task-detail-result">
        <div
          class="flex min-w-0 items-center gap-2 text-admin-body font-bold text-admin-title-color"
        >
          <MediaImage class="h-[17px] w-[17px] shrink-0 text-admin-subtitle-color" />
          <span id="task-detail-result">生成结果</span>
        </div>
        <div
          v-if="resultImageUrl"
          class="group relative overflow-hidden rounded-2xl border border-admin-border-soft bg-admin-preview-bg transition-colors hover:border-admin-border-hover"
        >
          <PrimeImage
            :src="resultImageUrl"
            preview
            image-class="block"
            :image-style="{ width: '100%', objectFit: 'contain' }"
          />
          <Button
            severity="secondary"
            size="small"
            label="在新窗口打开"
            class="!absolute !right-3 !bottom-3 !opacity-0 transition-opacity group-hover:!opacity-100 group-focus-within:!opacity-100 focus-visible:!opacity-100"
            @click="openResultImage(resultImageUrl)"
          />
        </div>
        <div
          v-else
          class="rounded-[14px] border border-admin-border-soft bg-admin-surface-muted/65 px-4 py-3 text-admin-body text-admin-text-3"
        >
          {{
            hasResultImageCandidate ? '结果图片链接不安全，已拒绝预览。' : '没有可预览的图片结果。'
          }}
        </div>
      </section>

      <section class="flex flex-col gap-2.5" aria-labelledby="task-detail-api-result">
        <div
          class="flex min-w-0 items-center gap-2 text-admin-body font-bold text-admin-title-color"
        >
          <CodeBrackets class="h-[17px] w-[17px] shrink-0 text-admin-subtitle-color" />
          <span id="task-detail-api-result">API 响应摘要</span>
          <Button
            text
            size="small"
            label="复制 JSON"
            :class="[adminTextButtonClass, 'ml-auto !text-admin-caption !text-admin-text-3']"
            :disabled="!hasResultPayload"
            @click="copyToClipboard(resultSummaryText)"
          />
        </div>
        <div
          class="overflow-x-auto rounded-[14px] border border-admin-border-soft bg-white/80 px-4 py-3.5 font-mono text-admin-meta"
        >
          <pre class="m-0 leading-[1.6] text-admin-text-2-strong">{{
            resultSummaryText || '暂无 API 响应结果'
          }}</pre>
        </div>
        <div
          v-if="resultDebugText"
          class="mt-2 overflow-hidden rounded-[14px] border border-admin-border-soft bg-admin-surface-muted/65"
        >
          <Button
            text
            size="small"
            :label="`原始调试详情 (脱敏) · ${showDebugResult ? '隐藏' : '展开'}`"
            :class="[
              adminTextButtonClass,
              'w-full !justify-start !rounded-none !px-3.5 !py-3 !text-admin-meta !font-semibold !text-admin-text-2-strong',
            ]"
            :aria-expanded="showDebugResult"
            aria-controls="task-result-debug-details"
            @click="showDebugResult = !showDebugResult"
          >
            <template #icon>
              <Terminal class="h-[17px] w-[17px] shrink-0 text-admin-subtitle-color" />
            </template>
          </Button>
          <div
            v-show="showDebugResult"
            id="task-result-debug-details"
            class="overflow-x-auto rounded-none border-0 border-t border-admin-border-soft bg-white/75 px-4 py-3.5 font-mono text-admin-meta"
          >
            <pre class="m-0 leading-[1.6] text-admin-text-3">{{ resultDebugText }}</pre>
          </div>
        </div>
      </section>
    </div>
  </Dialog>
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
  Terminal,
  WarningTriangle,
  XmarkCircle,
} from '@iconoir/vue'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import PrimeImage from 'primevue/image'
import Tag from 'primevue/tag'
import { formatIdeaAmount } from '../../../shared/utils/ideaCurrency'
import { adminOverlayDialogRootClass, createAdminDialogPt } from '../shared/lib/adminOverlayPt'
import { adminTextButtonClass } from '../shared/lib/adminUiClasses'
import {
  getTaskCredentialLabel,
  getTaskEndpointLabel,
  getTaskModelLabel,
  getTaskPriorityLabel,
} from './taskMonitorColumns'
import { adminTaskStatuses, getTaskStatusDefinition } from './taskMonitorStatus'
import type { AdminTask } from './taskMonitorResource'
import DetailMetric from './TaskResultDetailMetric.vue'
import { useAdminTaskResultFeedback } from './useAdminTaskMonitorFeedback'
import { useTaskResultDetail } from './useTaskResultDetail'

const props = defineProps<{ visible: boolean; task: AdminTask | null }>()
const emit = defineEmits<{ 'update:visible': [visible: boolean] }>()
const feedback = useAdminTaskResultFeedback()
const {
  detailTask,
  showDebugResult,
  resultImageUrl,
  resultSummaryText,
  resultDebugText,
  hasResultImageCandidate,
  hasResultPayload,
  openResultImage,
  copyToClipboard,
  formatDateTime,
  getErrorReason,
  calcDuration,
} = useTaskResultDetail(() => props.task, { feedback })

const taskDetailDialogPt = createAdminDialogPt({
  rootClass: adminOverlayDialogRootClass,
  headerClass:
    '!border-b !border-admin-border-soft !px-6 !pt-5 !pb-3.5 max-[720px]:!px-[18px] max-[720px]:!pt-4 max-[720px]:!pb-3',
  contentClass:
    '!max-h-[75vh] !overflow-y-auto !px-6 !pt-[18px] !pb-6 max-[720px]:!px-[18px] max-[720px]:!pt-3.5 max-[720px]:!pb-5',
})
const detailBannerBaseClass =
  'relative flex flex-col gap-3 overflow-hidden rounded-[18px] border border-admin-border-soft bg-admin-surface-muted px-[18px] py-4 before:absolute before:top-3.5 before:bottom-3.5 before:left-0 before:w-1 before:rounded-full before:bg-admin-text-3'
const detailStatusDotBaseClass = 'h-[9px] w-[9px] shrink-0 rounded-full'
const detailMetricClass =
  'flex min-w-0 flex-col gap-1 rounded-[14px] border border-admin-border bg-admin-surface-muted/70 px-3 py-2.5'

watch(
  () => props.visible,
  (visible) => {
    if (!visible) showDebugResult.value = false
  },
)

function handleVisibleUpdate(visible: boolean) {
  emit('update:visible', visible)
}
function getTaskStatusBannerClass(status: string) {
  return [detailBannerBaseClass, getTaskStatusDefinition(status).bannerClass]
}
function getTaskStatusDotClass(status: string) {
  return [detailStatusDotBaseClass, getTaskStatusDefinition(status).dotClass]
}
</script>
