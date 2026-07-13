<template>
  <div :class="adminDashboardRootClass">
    <Message v-if="taskMonitorSettingsError" severity="warn" :closable="false">
      {{ taskMonitorSettingsErrorMessage }}
    </Message>

    <Message v-if="isMonitorError" severity="error" :closable="false">
      {{ monitorErrorMessage }}
    </Message>

    <Message v-if="isMonitorStale" severity="warn" :closable="false">
      <div class="flex flex-wrap items-center gap-2">
        <span>任务监控连接不稳定，当前显示上次成功数据。</span>
        <Button
          text
          size="small"
          :class="adminTextButtonClass"
          label="重试连接"
          @click="retryMonitorSnapshot"
        />
      </div>
    </Message>

    <template v-if="!isMonitorError">
      <div :class="adminSummaryGridClass">
        <AdminSummaryCard label="执行中" :value="liveRunningCount" :footer="runningFoot" />
        <AdminSummaryCard label="队列等待" :value="liveQueueTotal" :footer="queueFoot" />
        <AdminSummaryCard label="失败" :value="liveFailedCount" :footer="failedFoot" />
        <AdminSummaryCard
          label="账号并发"
          :value="`${peakAccountLoadPercent}%`"
          :footer="accountLoadFoot"
        />
      </div>

      <div :class="adminChartGridClass">
        <AdminChartCard
          :title="taskCards.executionPressure.title"
          span-class="xl:col-span-12"
          chart-size="monitor"
        >
          <template #actions>
            <Tag
              :value="`${activeExecutionPressurePoints.length} 类`"
              severity="secondary"
              rounded
            />
          </template>
          <Chart
            type="bubble"
            :class="adminChartCanvasClass"
            :data="executionPressureChartData"
            :options="executionPressureChartOptions"
            :plugins="adminChartPlugins"
          />
        </AdminChartCard>

        <AdminChartCard
          :title="taskCards.taskDistribution.title"
          span-class="xl:col-span-6"
          chart-size="monitor"
        >
          <template #actions>
            <Tag :value="`${taskDistributionRows.length} 类`" severity="secondary" rounded />
          </template>
          <Chart
            type="radar"
            :class="adminChartCanvasClass"
            :data="taskDistributionChartData"
            :options="taskDistributionChartOptions"
            :plugins="adminChartPlugins"
          />
        </AdminChartCard>

        <AdminChartCard
          :title="taskCards.accountConcurrency.title"
          span-class="xl:col-span-6"
          chart-size="monitor"
        >
          <template #actions>
            <Tag :value="`${accountLoadRows.length} 账号`" severity="secondary" rounded />
          </template>
          <Chart
            type="bar"
            :class="adminChartCanvasClass"
            :data="accountConcurrencyChartData"
            :options="accountConcurrencyChartOptions"
            :plugins="adminChartPlugins"
          />
        </AdminChartCard>
      </div>
    </template>

    <Message v-if="isTaskListError" severity="error" :closable="false">
      {{ taskListErrorMessage }}
    </Message>

    <AdminTablePanel v-else>
      <template #toolbar>
        <AdminResourceToolbar>
          <template #search>
            <AdminToolbarSearch
              v-model="searchKeyword"
              @enter="handleTaskSearch"
              @clear="clearSearch"
            />
          </template>

          <template #metrics>
            <Tag :value="`${total} 任务`" severity="secondary" rounded />
            <Tag
              v-if="taskStats.pending > 0"
              :value="`${taskStats.pending} 等待中`"
              severity="secondary"
              rounded
            />
            <Tag
              v-if="taskStats.running > 0"
              :value="`${taskStats.running} 执行中`"
              severity="warn"
              rounded
            />
            <Tag
              v-if="taskStats.failed > 0"
              :value="`${taskStats.failed} 失败`"
              severity="danger"
              rounded
            />
            <Tag
              v-if="taskStats.completed > 0"
              :value="`${taskStats.completed} 已完成`"
              severity="success"
              rounded
            />
            <Tag
              v-if="taskStats.cancelled > 0"
              :value="`${taskStats.cancelled} 已取消`"
              severity="secondary"
              rounded
            />
            <Tag v-if="selectedTasks.length > 0" :value="`已选 ${selectedTasks.length}`" rounded />
          </template>

          <template #filters>
            <Button
              text
              :class="adminToolbarIconButtonClass"
              :disabled="!canDeleteSelectedTasks"
              :loading="isDeletingSelectedTasks"
              aria-label="删除"
              title="删除"
              @click="requestDeleteSelectedTasks"
            >
              <template #icon>
                <Trash class="size-4 text-admin-danger" />
              </template>
            </Button>
            <AdminFilterPopover
              :active="hasTaskFilter"
              filter-label="筛选任务"
              @reset="resetTaskFilters"
            >
              <label
                class="flex min-w-0 flex-col gap-1.5 text-admin-meta font-[650] text-admin-text-2"
              >
                <span>状态</span>
                <Select
                  v-model="filterStatus"
                  :options="taskStatusSelectOptions"
                  option-label="label"
                  option-value="value"
                  size="small"
                  fluid
                />
              </label>

              <label
                class="flex min-w-0 flex-col gap-1.5 text-admin-meta font-[650] text-admin-text-2"
              >
                <span>账号 ID</span>
                <InputNumber
                  v-model="accountIdFilter"
                  :min="1"
                  :use-grouping="false"
                  placeholder="Account ID"
                  size="small"
                  fluid
                />
              </label>
            </AdminFilterPopover>
          </template>

          <template #actions="{ iconButtonClass }">
            <Button
              text
              :class="iconButtonClass"
              aria-label="刷新任务列表"
              :disabled="isDeletingSelectedTasks"
              @click="loadTasks"
            >
              <template #icon>
                <Refresh class="size-4 text-admin-text-2" />
              </template>
            </Button>
          </template>

          <template #pager>
            <AdminPagerControls
              :current-page="page"
              :total-pages="totalPages"
              :prev-disabled="page === 1 || loading"
              :next-disabled="page === totalPages || loading"
              @prev="handlePrevTaskPage"
              @next="handleNextTaskPage"
            />
          </template>
        </AdminResourceToolbar>
      </template>

      <div class="app-scrollbar overflow-x-auto overscroll-contain">
        <DataTable
          v-model:selection="selectedTasks"
          :value="tasks"
          :loading="loading"
          data-key="task_id"
          size="small"
          :pt="adminDenseDataTablePt"
          :table-style="{ minWidth: '1536px' }"
        >
          <template #empty>
            <div class="py-7 text-center text-admin-body text-admin-text-3">暂无任务</div>
          </template>

          <Column
            selection-mode="multiple"
            header-style="width: 3.5rem"
            :body-style="{ width: '3.5rem' }"
          />

          <Column header="创建时间" :style="{ width: '156px' }">
            <template #body="{ data }">
              <span
                class="whitespace-nowrap font-mono text-admin-meta font-semibold text-admin-text-2-strong"
              >
                {{ formatTaskCreatedAt(data) }}
              </span>
            </template>
          </Column>

          <Column header="结果" :style="{ width: '104px', minWidth: '104px' }">
            <template #body="{ data }">
              <div class="flex h-12 w-12 shrink-0 items-center justify-start">
                <PrimeImage
                  v-if="resultPreviewEnabled && getTaskResultImageUrl(data)"
                  :src="getTaskResultImageUrl(data)"
                  preview
                  image-class="block h-12 w-12 min-w-12 shrink-0 overflow-hidden rounded-[10px] border border-admin-border-soft bg-admin-surface-muted object-cover"
                  :image-style="{
                    width: '48px',
                    height: '48px',
                    minWidth: '48px',
                    objectFit: 'cover',
                  }"
                />
                <span v-else class="text-admin-meta text-admin-text-3">-</span>
              </div>
            </template>
          </Column>

          <Column header="用户" :style="{ width: '140px' }">
            <template #body="{ data }">
              <span
                class="block max-w-[120px] truncate text-admin-meta font-bold text-admin-text-1"
              >
                {{ getTaskUserIdentifier(data) }}
              </span>
            </template>
          </Column>

          <Column header="提示词" :style="{ width: '280px' }">
            <template #body="{ data }">
              <span
                class="block max-w-[252px] truncate text-admin-meta text-admin-text-2-strong"
                :title="data.prompt || '-'"
              >
                {{ data.prompt || '-' }}
              </span>
            </template>
          </Column>

          <Column header="账号" :style="{ width: '88px' }">
            <template #body="{ data }">
              <span class="text-admin-meta text-admin-text-3">{{ data.account_id || '-' }}</span>
            </template>
          </Column>

          <Column header="Endpoint" :style="{ width: '184px' }">
            <template #body="{ data }">
              <div class="flex min-w-0 flex-col gap-0.5">
                <span
                  class="block max-w-[160px] truncate text-admin-meta font-semibold text-admin-text-2-strong"
                  :title="data.provider_endpoint_base_url || getTaskEndpointLabel(data) || '-'"
                >
                  {{ getTaskEndpointLabel(data) || '-' }}
                </span>
                <span class="block max-w-[160px] truncate text-admin-micro text-admin-text-3">
                  {{ data.api_type || '-' }}
                </span>
              </div>
            </template>
          </Column>

          <Column header="模型" :style="{ width: '156px' }">
            <template #body="{ data }">
              <span
                class="block max-w-[132px] truncate text-admin-meta font-semibold text-admin-text-2-strong"
                :title="getTaskModelLabel(data) || '-'"
              >
                {{ getTaskModelLabel(data) || '-' }}
              </span>
            </template>
          </Column>

          <Column header="优先级" :style="{ width: '88px' }">
            <template #body="{ data }">
              <Tag :value="getTaskPriorityLabel(data)" severity="secondary" rounded />
            </template>
          </Column>

          <Column header="API调用" :style="{ width: '144px' }">
            <template #body="{ data }">
              <span v-if="getTaskApiCall(data)" class="inline-flex max-w-32 items-center gap-1.5">
                <Tag :value="getTaskApiCall(data)?.platform" severity="info" rounded />
              </span>
              <span v-else class="text-admin-meta text-admin-text-3">-</span>
            </template>
          </Column>

          <Column header="状态" :style="{ width: '104px' }">
            <template #body="{ data }">
              <Tag
                :value="getTaskStatusDefinition(data.status).text"
                :severity="getTaskStatusDefinition(data.status).severity"
                rounded
                class="min-w-16 justify-center"
              />
            </template>
          </Column>

          <Column header="耗时" :style="{ width: '88px' }">
            <template #body="{ data }">
              <span v-if="getTaskDuration(data)" :class="getTaskDurationClass(data)">
                {{ getTaskDuration(data)?.seconds }}s
              </span>
              <span v-else class="text-admin-meta text-admin-text-3">-</span>
            </template>
          </Column>

          <Column header="错误诊断" :style="{ width: '240px' }">
            <template #body="{ data }">
              <span
                v-if="getTaskSanitizedError(data)"
                class="block max-w-[212px] truncate text-admin-meta text-admin-danger"
              >
                {{ getTaskSanitizedError(data) }}
              </span>
              <span v-else class="text-admin-meta text-admin-text-3">-</span>
            </template>
          </Column>

          <Column header="操作" :style="{ width: '72px' }">
            <template #body="{ data }">
              <Button
                text
                class="!h-8 !w-8 !rounded-none !border-0 !bg-transparent !p-0 !shadow-none hover:!bg-admin-surface-muted"
                aria-label="查看任务详情"
                @click="showDetail(data)"
              >
                <template #icon>
                  <Eye class="size-4 text-admin-text-2" />
                </template>
              </Button>
            </template>
          </Column>
        </DataTable>
      </div>
    </AdminTablePanel>

    <Dialog
      v-model:visible="detailModalVisible"
      modal
      :style="{ width: '680px', maxWidth: '95vw' }"
      :pt="taskDetailDialogPt"
      :draggable="false"
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
                ID: {{ detailTask?.task_id }}
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
              @click="copyToClipboard(detailTask?.task_id || '')"
            >
              <template #icon>
                <Copy />
              </template>
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
            v-else
            class="grid grid-cols-1 gap-2.5 border-t border-admin-divider pt-3 md:grid-cols-2"
          >
            <div
              class="flex min-w-0 flex-col gap-1 rounded-[13px] border border-admin-border bg-white/70 px-3 py-2.5"
            >
              <span class="text-admin-micro font-bold text-admin-text-3">总耗时</span>
              <span class="truncate text-admin-body font-bold text-admin-text-2-strong">
                {{ calcDuration(detailTask) }}
              </span>
            </div>
            <div
              class="flex min-w-0 flex-col gap-1 rounded-[13px] border border-admin-border bg-white/70 px-3 py-2.5"
            >
              <span class="text-admin-micro font-bold text-admin-text-3">创建于</span>
              <span class="truncate text-admin-body font-bold text-admin-text-2-strong">
                {{ formatDateTime(detailTask.created_at).split('-')[1] }}
              </span>
            </div>
          </div>
        </div>

        <div class="flex flex-col gap-2.5">
          <div
            class="flex min-w-0 items-center gap-2 text-admin-body font-bold text-admin-title-color"
          >
            <Fingerprint class="h-[17px] w-[17px] shrink-0 text-admin-subtitle-color" />
            <span>身份与归属</span>
          </div>
          <div class="grid grid-cols-1 gap-2.5 md:grid-cols-2">
            <div
              class="flex min-w-0 flex-col gap-1 rounded-[14px] border border-admin-border bg-admin-surface-muted/70 px-3 py-2.5"
            >
              <span class="text-admin-caption font-bold text-admin-text-3">用户 ID</span>
              <div class="flex min-w-0 items-center justify-between gap-2">
                <span class="min-w-0 truncate text-admin-body font-semibold text-admin-text-1">
                  {{ detailTask.user_id }}
                </span>
                <Button
                  text
                  rounded
                  size="small"
                  :class="adminTextButtonClass"
                  aria-label="复制用户 ID"
                  @click="copyToClipboard(String(detailTask.user_id))"
                >
                  <template #icon>
                    <Copy />
                  </template>
                </Button>
              </div>
            </div>
            <div
              class="flex min-w-0 flex-col gap-1 rounded-[14px] border border-admin-border bg-admin-surface-muted/70 px-3 py-2.5"
            >
              <span class="text-admin-caption font-bold text-admin-text-3">优先级</span>
              <span class="min-w-0 truncate text-admin-body font-semibold text-admin-text-1">
                {{ getTaskPriorityLabel(detailTask) }}
              </span>
            </div>
            <div
              class="flex min-w-0 flex-col gap-1 rounded-[14px] border border-admin-border bg-admin-surface-muted/70 px-3 py-2.5"
            >
              <span class="text-admin-caption font-bold text-admin-text-3">账号 ID</span>
              <span class="min-w-0 truncate text-admin-body font-semibold text-admin-text-1">
                {{ detailTask.account_id || 'N/A' }}
              </span>
            </div>
            <div
              class="flex min-w-0 flex-col gap-1 rounded-[14px] border border-admin-border bg-admin-surface-muted/70 px-3 py-2.5"
            >
              <span class="text-admin-caption font-bold text-admin-text-3">Endpoint</span>
              <span
                class="min-w-0 truncate text-admin-body font-semibold text-admin-text-1"
                :title="detailTask.provider_endpoint_base_url || getTaskEndpointLabel(detailTask)"
              >
                {{ getTaskEndpointLabel(detailTask) || 'N/A' }}
              </span>
            </div>
            <div
              class="flex min-w-0 flex-col gap-1 rounded-[14px] border border-admin-border bg-admin-surface-muted/70 px-3 py-2.5"
            >
              <span class="text-admin-caption font-bold text-admin-text-3">Credential</span>
              <span
                class="min-w-0 truncate text-admin-body font-semibold text-admin-text-1"
                :title="getTaskCredentialLabel(detailTask) || 'N/A'"
              >
                {{ getTaskCredentialLabel(detailTask) || 'N/A' }}
              </span>
            </div>
            <div
              class="flex min-w-0 flex-col gap-1 rounded-[14px] border border-admin-border bg-admin-surface-muted/70 px-3 py-2.5"
            >
              <span class="text-admin-caption font-bold text-admin-text-3">模型 / API Type</span>
              <span
                class="min-w-0 truncate text-admin-body font-semibold text-admin-text-1"
                :title="`${getTaskModelLabel(detailTask) || 'N/A'} · ${detailTask.api_type || 'N/A'}`"
              >
                {{ getTaskModelLabel(detailTask) || 'N/A' }} · {{ detailTask.api_type || 'N/A' }}
              </span>
            </div>
            <div
              class="flex min-w-0 flex-col gap-1 rounded-[14px] border border-admin-border bg-admin-surface-muted/70 px-3 py-2.5"
            >
              <span class="text-admin-caption font-bold text-admin-text-3">创建时间</span>
              <span class="min-w-0 truncate text-admin-body font-semibold text-admin-text-1">
                {{ formatDateTime(detailTask.created_at) }}
              </span>
            </div>
            <div
              class="flex min-w-0 flex-col gap-1 rounded-[14px] border border-admin-border bg-admin-surface-muted/70 px-3 py-2.5"
            >
              <span class="text-admin-caption font-bold text-admin-text-3">完成时间</span>
              <span class="min-w-0 truncate text-admin-body font-semibold text-admin-text-1">
                {{ formatDateTime(detailTask.completed_at) }}
              </span>
            </div>
          </div>
        </div>

        <div class="flex flex-col gap-2.5">
          <div
            class="flex min-w-0 items-center gap-2 text-admin-body font-bold text-admin-title-color"
          >
            <ChatBubble class="h-[17px] w-[17px] shrink-0 text-admin-subtitle-color" />
            <span>任务指令 (Prompt)</span>
            <Button
              text
              size="small"
              label="复制全文"
              :class="[adminTextButtonClass, 'ml-auto !text-admin-caption !text-admin-text-3']"
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
        </div>

        <div v-if="resultImageUrl" class="flex flex-col gap-2.5">
          <div
            class="flex min-w-0 items-center gap-2 text-admin-body font-bold text-admin-title-color"
          >
            <MediaImage class="h-[17px] w-[17px] shrink-0 text-admin-subtitle-color" />
            <span>生成结果</span>
          </div>
          <div
            class="group relative overflow-hidden rounded-2xl border border-admin-border-soft bg-admin-preview-bg transition-colors hover:border-admin-border-hover"
          >
            <PrimeImage
              :src="resultImageUrl"
              preview
              image-class="block"
              :image-style="{ width: '100%', objectFit: 'contain' }"
            />
            <div
              class="absolute right-3 bottom-3 opacity-0 transition-opacity group-hover:opacity-100"
            >
              <Button
                severity="secondary"
                size="small"
                label="在新窗口打开"
                @click="openResultImage(resultImageUrl)"
              />
            </div>
          </div>
        </div>

        <div class="flex flex-col gap-2.5">
          <div
            class="flex min-w-0 items-center gap-2 text-admin-body font-bold text-admin-title-color"
          >
            <CodeBrackets class="h-[17px] w-[17px] shrink-0 text-admin-subtitle-color" />
            <span>API 响应摘要</span>
            <Button
              text
              size="small"
              label="复制 JSON"
              :class="[adminTextButtonClass, 'ml-auto !text-admin-caption !text-admin-text-3']"
              @click="copyToClipboard(resultSummaryText)"
            />
          </div>
          <div
            class="overflow-x-auto rounded-[14px] border border-admin-border-soft bg-white/80 px-4 py-3.5 font-mono text-admin-meta"
          >
            <pre class="m-0 leading-[1.6] text-admin-text-2-strong">{{ resultSummaryText }}</pre>
          </div>

          <div
            v-if="resultDebugText"
            class="mt-2 overflow-hidden rounded-[14px] border border-admin-border-soft bg-admin-surface-muted/65"
          >
            <div
              class="flex cursor-pointer items-center justify-between gap-3 px-3.5 py-3"
              @click="showDebugResult = !showDebugResult"
            >
              <div
                class="flex items-center gap-2 text-admin-meta font-semibold text-admin-text-2-strong"
              >
                <Terminal class="h-[17px] w-[17px] shrink-0 text-admin-subtitle-color" />
                <span>原始调试详情 (脱敏)</span>
              </div>
              <Button
                text
                size="small"
                :label="showDebugResult ? '隐藏' : '展开'"
                :class="adminTextButtonClass"
                @click.stop="showDebugResult = !showDebugResult"
              />
            </div>
            <div
              v-show="showDebugResult"
              class="overflow-x-auto rounded-none border-0 border-t border-admin-border-soft bg-white/75 px-4 py-3.5 font-mono text-admin-meta"
            >
              <pre class="m-0 leading-[1.6] text-admin-text-3">{{ resultDebugText }}</pre>
            </div>
          </div>
        </div>
      </div>
    </Dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import Button from 'primevue/button'
import Chart from 'primevue/chart'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import Dialog from 'primevue/dialog'
import InputNumber from 'primevue/inputnumber'
import Message from 'primevue/message'
import PrimeImage from 'primevue/image'
import Select from 'primevue/select'
import Tag from 'primevue/tag'
import {
  ChatBubble,
  CheckCircle,
  Clock,
  CodeBrackets,
  Copy,
  Eye,
  Fingerprint,
  MediaImage,
  Refresh,
  Terminal,
  Trash,
  WarningTriangle,
  XmarkCircle,
} from '@iconoir/vue'
import { formatDisplayErrorMessage } from '../../../../shared/utils/errorMessages'
import { useAdminFeedback } from '../../feedback/useAdminFeedback'
import AdminChartCard from '../../shared/components/AdminChartCard.vue'
import AdminFilterPopover from '../../shared/components/AdminFilterPopover.vue'
import AdminPagerControls from '../../shared/components/AdminPagerControls.vue'
import AdminResourceToolbar from '../../shared/components/AdminResourceToolbar.vue'
import AdminSummaryCard from '../../shared/components/AdminSummaryCard.vue'
import AdminTablePanel from '../../shared/components/AdminTablePanel.vue'
import AdminToolbarSearch from '../../shared/components/AdminToolbarSearch.vue'
import { adminCards } from '../../shared/lib/adminCardMeta'
import { adminChartPlugins } from '../../shared/lib/adminChart'
import { adminDenseDataTablePt } from '../../shared/lib/adminDataTablePt'
import { createAdminDialogPt } from '../../shared/lib/adminOverlayPt'
import {
  adminChartCanvasClass,
  adminChartGridClass,
  adminDashboardRootClass,
  adminSummaryGridClass,
  adminTextButtonClass,
  adminToolbarIconButtonClass,
} from '../../shared/lib/adminUiClasses'
import { useAdminConfirm } from '../../shared/composables/useAdminConfirm'
import {
  formatTaskCreatedAt,
  getTaskApiCall,
  getTaskDuration,
  getTaskCredentialLabel,
  getTaskEndpointLabel,
  getTaskModelLabel,
  getTaskPriorityLabel,
  getTaskResultImageUrl,
  getTaskSanitizedError,
  getTaskUserIdentifier,
} from '../../task-monitor/taskMonitorColumns'
import {
  adminTaskStatuses,
  getTaskStatusDefinition,
  taskStatusSelectOptions,
} from '../../task-monitor/taskMonitorStatus'
import { useTaskResultDetail } from '../../task-monitor/useTaskResultDetail'
import { useTaskMonitorCharts } from '../../task-monitor/useTaskMonitorCharts'
import { type AdminTaskRow, useAdminTaskList } from '../../task-monitor/useAdminTaskList'
import { useAdminTaskSelectionDelete } from '../../task-monitor/useAdminTaskSelectionDelete'
import { useAdminTaskRuntimeSnapshot } from '../../task-monitor/useAdminTaskRuntimeSnapshot'

const adminFeedback = useAdminFeedback()
const adminConfirm = useAdminConfirm()
const taskCards = adminCards.taskMonitor
const taskDetailDialogPt = createAdminDialogPt({
  rootClass:
    '!overflow-hidden !border !border-[rgba(215,224,235,0.92)] !bg-[linear-gradient(180deg,rgba(255,255,255,0.99),rgba(247,250,253,0.96))] !shadow-[0_24px_60px_rgba(45,64,89,0.16)]',
  headerClass:
    '!border-b !border-admin-border-soft !px-6 !pt-5 !pb-3.5 max-[720px]:!px-[18px] max-[720px]:!pt-4 max-[720px]:!pb-3',
  contentClass:
    '!max-h-[75vh] !overflow-y-auto !px-6 !pt-[18px] !pb-6 max-[720px]:!px-[18px] max-[720px]:!pt-3.5 max-[720px]:!pb-5',
})
const detailBannerBaseClass =
  'relative flex flex-col gap-3 overflow-hidden rounded-[18px] border border-admin-border-soft bg-admin-surface-muted px-[18px] py-4 before:absolute before:top-3.5 before:bottom-3.5 before:left-0 before:w-1 before:rounded-full before:bg-admin-text-3'
const detailStatusDotBaseClass =
  'h-[9px] w-[9px] shrink-0 rounded-full bg-admin-text-3 shadow-[0_0_0_4px_rgba(152,162,179,0.14)]'

const {
  detailModalVisible,
  detailTask,
  showDebugResult,
  resultImageUrl,
  resultSummaryText,
  resultDebugText,
  openResultImage,
  copyToClipboard,
  formatDateTime,
  showDetail,
  getErrorReason,
  calcDuration,
} = useTaskResultDetail({ feedback: adminFeedback.taskMonitor.resultDetail })

const {
  tasks,
  total,
  loading,
  isError: isTaskListError,
  error: taskListError,
  page,
  pageSize,
  resultPreviewEnabled,
  taskMonitorSettingsError,
  taskStats,
  filterStatus,
  accountIdFilter,
  searchKeyword,
  loadTasks,
  handleSearch: applyTaskSearch,
} = useAdminTaskList()
const {
  monitorData,
  monitorStats,
  monitorStreamStatus,
  isStale: isMonitorStale,
  isError: isMonitorError,
  error: monitorError,
  retry: retryMonitorSnapshot,
} = useAdminTaskRuntimeSnapshot()

const selectedTasks = ref<AdminTaskRow[]>([])
const { canDeleteSelectedTasks, isDeletingSelectedTasks, requestDeleteSelectedTasks } =
  useAdminTaskSelectionDelete({
    selectedTasks,
    confirm: adminConfirm,
    feedback: adminFeedback.taskMonitor.taskDeletion,
  })
const hasTaskFilter = computed(() => Boolean(filterStatus.value || accountIdFilter.value))
const totalPages = computed(() => Math.ceil(total.value / pageSize.value) || 1)
const taskListErrorMessage = computed(() =>
  formatDisplayErrorMessage(taskListError.value, '加载任务列表失败'),
)
const monitorErrorMessage = computed(() =>
  formatDisplayErrorMessage(monitorError.value, '加载任务运行监控失败'),
)
const taskMonitorSettingsErrorMessage = computed(() =>
  formatDisplayErrorMessage(
    taskMonitorSettingsError.value,
    '加载任务监控设置失败，当前使用默认展示配置',
  ),
)

watch(totalPages, (newTotal) => {
  if (page.value > newTotal) {
    page.value = Math.max(1, newTotal)
  }
})

function handlePrevTaskPage() {
  if (loading.value) return
  if (page.value > 1) {
    page.value--
  }
}

function handleNextTaskPage() {
  if (loading.value) return
  if (page.value < totalPages.value) {
    page.value++
  }
}

function handleTaskSearch() {
  selectedTasks.value = []
  applyTaskSearch()
}

function resetTaskFilters() {
  filterStatus.value = null
  accountIdFilter.value = null
}

watch([page, filterStatus, accountIdFilter], () => {
  selectedTasks.value = []
})

const currentPageCount = computed(() => tasks.value.length)
const {
  taskStatusSnapshot,
  statusTotalCount,
  accountLoadRows,
  accountLoadCurrent,
  accountLoadCapacity,
  peakAccountLoadPercent,
  taskDistributionRows,
  activeExecutionPressurePoints,
  accountConcurrencyChartData,
  accountConcurrencyChartOptions,
  taskDistributionChartData,
  taskDistributionChartOptions,
  executionPressureChartData,
  executionPressureChartOptions,
} = useTaskMonitorCharts({ monitorData, monitorStats })
const liveRunningCount = computed(() => monitorData.value?.running_attempts ?? 0)
const liveFailedCount = computed(() => taskStatusSnapshot.value[adminTaskStatuses.failed])
const globalQueueLength = computed(() => monitorData.value?.global_queue_len || 0)
const waitQueueLength = computed(() => monitorData.value?.wait_queue_len || 0)
const liveQueueTotal = computed(() => globalQueueLength.value + waitQueueLength.value)
const runningFoot = computed(() => formatMonitorStreamStatus(monitorStreamStatus.value))
const queueFoot = computed(() =>
  waitQueueLength.value > 0
    ? `全局 ${globalQueueLength.value} / 等待 ${waitQueueLength.value}`
    : `全局 ${globalQueueLength.value}`,
)
const failedFoot = computed(
  () => `状态占比 ${formatPercent(liveFailedCount.value, statusTotalCount.value)}`,
)
const accountLoadFoot = computed(() =>
  accountLoadRows.value.length > 0
    ? `${accountLoadCurrent.value}/${accountLoadCapacity.value} 并发占用`
    : `当前页 ${currentPageCount.value} / 查询 ${total.value}`,
)

function clearSearch() {
  searchKeyword.value = ''
  handleTaskSearch()
}

function getTaskStatusBannerClass(status: string) {
  return [detailBannerBaseClass, getTaskStatusDefinition(status).bannerClass]
}

function getTaskStatusDotClass(status: string) {
  return [detailStatusDotBaseClass, getTaskStatusDefinition(status).dotClass]
}

function getTaskDurationClass(task: Parameters<typeof getTaskDuration>[0]) {
  const tone = getTaskDuration(task)?.tone
  return [
    'font-mono text-admin-meta font-bold',
    tone === 'success' && 'text-admin-success',
    tone === 'warning' && 'text-admin-warning',
    tone === 'danger' && 'text-admin-danger',
  ]
}

function formatPercent(value: number, totalValue: number) {
  if (totalValue <= 0) return '0.0%'
  return `${((value / totalValue) * 100).toFixed(1)}%`
}

function formatMonitorStreamStatus(status: string) {
  const textByStatus: Record<string, string> = {
    idle: '实时流待命',
    unsupported: '实时流不可用',
    connecting: '实时流连接中',
    connected: '实时流已连接',
    degraded: '实时流降级',
  }
  return textByStatus[status] || '实时流未知'
}
</script>
