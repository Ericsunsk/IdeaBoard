<template>
  <div class="flex max-w-full min-w-0 flex-col gap-4 p-0">
    <div class="grid grid-cols-1 gap-3.5 md:grid-cols-2 xl:grid-cols-4">
      <Card>
        <template #subtitle>
          <span class="text-[13px] font-bold text-admin-subtitle-color tracking-normal"
            >执行中</span
          >
        </template>
        <template #title>
          <span class="text-[26px] font-[780] text-admin-title-color leading-[1.05]">
            {{ pageStats.running }}
          </span>
        </template>
        <template #content>{{ runningFoot }}</template>
      </Card>
      <Card>
        <template #subtitle>
          <span class="text-[13px] font-bold text-admin-subtitle-color tracking-normal"
            >排队中</span
          >
        </template>
        <template #title>
          <span class="text-[26px] font-[780] text-admin-title-color leading-[1.05]">
            {{ pageStats.pending }}
          </span>
        </template>
        <template #content>{{ pendingFoot }}</template>
      </Card>
      <Card>
        <template #subtitle>
          <span class="text-[13px] font-bold text-admin-subtitle-color tracking-normal">失败</span>
        </template>
        <template #title>
          <span class="text-[26px] font-[780] text-admin-title-color leading-[1.05]">
            {{ pageStats.failed }}
          </span>
        </template>
        <template #content>{{ failedFoot }}</template>
      </Card>
      <Card>
        <template #subtitle>
          <span class="text-[13px] font-bold text-admin-subtitle-color tracking-normal">其他</span>
        </template>
        <template #title>
          <span class="text-[26px] font-[780] text-admin-title-color leading-[1.05]">
            {{ summaryOtherCount }}
          </span>
        </template>
        <template #content>{{ otherFoot }}</template>
      </Card>
    </div>

    <div class="grid grid-cols-1 gap-3.5 xl:grid-cols-12">
      <Card class="xl:col-span-5">
        <template #title>
          <div class="flex items-start justify-between gap-3">
            <span>慢任务 Top</span>
            <Tag :value="`${slowTasks.length} 条`" severity="secondary" rounded />
          </div>
        </template>
        <template #subtitle>当前页耗时超过 60s</template>
        <template #content>
          <div v-if="slowTasks.length" class="h-[174px] min-w-0">
            <Chart
              type="bar"
              class="h-full w-full min-w-0"
              :data="slowTaskChartData"
              :options="slowTaskChartOptions"
              :plugins="adminChartPlugins"
              @select="handleSlowTaskChartSelect"
            />
          </div>
          <Message v-else severity="secondary" :closable="false">暂无慢任务</Message>
        </template>
      </Card>

      <Card class="xl:col-span-3">
        <template #title>
          <div class="flex items-start justify-between gap-3">
            <span>活跃任务龄</span>
            <Tag :value="`${activeTasks.length} 条`" severity="secondary" rounded />
          </div>
        </template>
        <template #subtitle>排队与执行中的等待时间</template>
        <template #content>
          <div v-if="activeTasks.length" class="flex flex-col gap-2">
            <Button
              v-for="row in activeTasks"
              :key="row.task.task_id"
              type="button"
              text
              severity="secondary"
              class="w-full justify-start"
              @click="showDetail(row.task)"
            >
              <span class="min-w-0 truncate font-semibold">{{ row.task.task_id }}</span>
              <span class="ml-auto shrink-0 text-xs text-admin-text-3">
                {{ getTaskStatusConfig(row.task.status).text }}
              </span>
              <span class="shrink-0 font-mono text-xs text-admin-info">
                {{ formatSeconds(row.seconds) }}
              </span>
            </Button>
          </div>
          <Message v-else severity="secondary" :closable="false">暂无活跃任务</Message>
        </template>
      </Card>

      <Card class="xl:col-span-4">
        <template #title>
          <div class="flex items-start justify-between gap-3">
            <span>失败样本</span>
            <Tag :value="`${failedTasks.length} 条`" severity="danger" rounded />
          </div>
        </template>
        <template #subtitle>当前页失败原因速览</template>
        <template #content>
          <div v-if="failedTasks.length" class="flex flex-col gap-2">
            <Button
              v-for="task in failedTasks.slice(0, 5)"
              :key="task.task_id"
              type="button"
              text
              severity="danger"
              class="w-full justify-start"
              @click="showDetail(task)"
            >
              <span class="min-w-0 truncate font-semibold">{{ task.task_id }}</span>
              <span class="ml-auto min-w-0 truncate text-xs">
                {{ getErrorReason(task) || '-' }}
              </span>
            </Button>
          </div>
          <Message v-else severity="secondary" :closable="false">暂无失败样本</Message>
        </template>
      </Card>
    </div>

    <div class="flex flex-wrap items-center justify-between gap-4 bg-transparent px-0 py-1">
      <!-- Left side -->
      <div class="flex flex-wrap items-center gap-3">
        <IconField class="relative w-full sm:w-[260px]">
          <InputIcon>
            <Search class="size-4 text-admin-text-3" />
          </InputIcon>
          <InputText
            v-model="searchKeyword"
            placeholder="搜索..."
            fluid
            class="!rounded-lg !border-admin-border"
            @keyup.enter="handleSearch"
          />
          <Button
            v-if="searchKeyword"
            text
            rounded
            class="absolute top-1/2 right-1 !h-8 !w-8 -translate-y-1/2"
            aria-label="清空搜索"
            @click="clearSearch"
          >
            <template #icon>
              <Xmark />
            </template>
          </Button>
        </IconField>
        <Select
          v-model="filterStatus"
          class="w-[152px] !rounded-lg"
          :options="taskStatusOptions"
          option-label="label"
          option-value="value"
          placeholder="所有状态"
          show-clear
        />
      </div>

      <!-- Right side -->
      <div class="flex items-center gap-3">
        <!-- Separator -->
        <div class="h-5 w-[1px] bg-admin-border-soft"></div>

        <!-- Refresh Button -->
        <Button
          text
          class="!h-9 !w-9 !rounded-lg border border-admin-border bg-white hover:bg-admin-surface-muted transition-colors flex items-center justify-center p-0"
          aria-label="刷新任务列表"
          @click="loadTasks"
        >
          <template #icon>
            <Refresh class="size-4 text-admin-text-2" />
          </template>
        </Button>

        <Paginator
          :first="paginatorFirst"
          :rows="TASK_MONITOR_PAGE_SIZE"
          :total-records="total"
          :disabled="loading"
          template="PrevPageLink CurrentPageReport NextPageLink"
          current-page-report-template="{currentPage} / {totalPages}"
          :pt="taskPaginatorPt"
          @page="handlePageChange"
        />
      </div>
    </div>

    <div
      class="min-w-0 overflow-hidden rounded-[20px] border border-admin-border-soft bg-admin-card-strong p-0 shadow-none"
    >
      <DataTable
        :value="tasks"
        :loading="loading"
        data-key="task_id"
        size="small"
        :pt="adminDenseDataTablePt"
        :table-style="{ minWidth: '1160px' }"
      >
        <template #empty>
          <div class="py-7 text-center text-admin-body text-admin-text-3">暂无任务</div>
        </template>

        <Column header="创建时间" :style="{ width: '150px' }">
          <template #body="{ data }">
            <span
              class="whitespace-nowrap font-mono text-admin-meta font-semibold text-admin-text-2-strong"
            >
              {{ formatTaskCreatedAt(data) }}
            </span>
          </template>
        </Column>

        <Column header="结果" :style="{ width: '76px' }" :body-style="{ textAlign: 'center' }">
          <template #body="{ data }">
            <PrimeImage
              v-if="getTaskResultImageUrl(data)"
              :src="getTaskResultImageUrl(data)"
              preview
              image-class="block overflow-hidden rounded-[10px] border border-admin-border-soft bg-admin-surface-muted"
              :image-style="{ width: '42px', height: '42px', objectFit: 'cover' }"
            />
            <span v-else class="text-admin-meta text-admin-text-3">-</span>
          </template>
        </Column>

        <Column header="用户" :style="{ width: '132px' }">
          <template #body="{ data }">
            <span class="block max-w-[116px] truncate text-admin-meta font-bold text-admin-text-1">
              {{ getTaskUserIdentifier(data) }}
            </span>
          </template>
        </Column>

        <Column header="提示词" :style="{ width: '260px' }">
          <template #body="{ data }">
            <span
              class="block max-w-[244px] truncate text-admin-meta text-admin-text-2-strong"
              :title="data.prompt || '-'"
            >
              {{ data.prompt || '-' }}
            </span>
          </template>
        </Column>

        <Column header="账号" :style="{ width: '80px' }" :body-style="{ textAlign: 'center' }">
          <template #body="{ data }">
            <span class="text-admin-meta text-admin-text-3">{{ data.account_id || '-' }}</span>
          </template>
        </Column>

        <Column header="API调用" :style="{ width: '140px' }">
          <template #body="{ data }">
            <span v-if="getTaskApiCall(data)" class="inline-flex max-w-32 items-center gap-1.5">
              <Tag :value="getTaskApiCall(data)?.platform" severity="info" rounded />
              <span
                v-if="getTaskApiCall(data)?.modelSet"
                class="block max-w-[70px] truncate text-admin-caption text-admin-text-3"
              >
                {{ getTaskApiCall(data)?.modelSet }}
              </span>
            </span>
            <span v-else class="text-admin-meta text-admin-text-3">-</span>
          </template>
        </Column>

        <Column header="状态" :style="{ width: '94px' }">
          <template #body="{ data }">
            <Tag
              :value="getTaskStatusConfig(data.status).text"
              :severity="getTaskStatusConfig(data.status).severity"
              rounded
              class="min-w-16 justify-center"
            />
          </template>
        </Column>

        <Column header="耗时" :style="{ width: '80px' }" :body-style="{ textAlign: 'center' }">
          <template #body="{ data }">
            <span v-if="getTaskDuration(data)" :class="getTaskDurationClass(data)">
              {{ getTaskDuration(data)?.seconds }}s
            </span>
            <span v-else class="text-admin-meta text-admin-text-3">-</span>
          </template>
        </Column>

        <Column header="错误诊断" :style="{ width: '220px' }">
          <template #body="{ data }">
            <span
              v-if="getTaskSanitizedError(data)"
              class="block max-w-[210px] truncate text-admin-meta text-admin-danger"
            >
              {{ getTaskSanitizedError(data) }}
            </span>
            <span v-else class="text-admin-meta text-admin-text-3">-</span>
          </template>
        </Column>

        <Column header="操作" :style="{ width: '88px' }" :body-style="{ textAlign: 'center' }">
          <template #body="{ data }">
            <Button size="small" text rounded label="详情" @click="showDetail(data)">
              <template #icon>
                <Eye />
              </template>
            </Button>
          </template>
        </Column>
      </DataTable>
    </div>

    <Dialog
      v-model:visible="detailModalVisible"
      modal
      :style="{ width: '680px', maxWidth: '95vw' }"
      :pt="detailDialogPt"
      :draggable="false"
    >
      <template #header>
        <div class="flex w-full items-center justify-between gap-3.5">
          <div class="flex min-w-0 items-center gap-3">
            <span
              class="inline-flex h-[38px] w-[38px] shrink-0 items-center justify-center rounded-[14px] border border-admin-border-soft bg-admin-surface-muted"
              :style="{ color: getTaskStatusConfig(detailTask?.status || '').color }"
            >
              <CheckCircle v-if="detailTask?.status === 'completed'" />
              <XmarkCircle v-else-if="detailTask?.status === 'failed'" />
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
              :value="getTaskStatusConfig(detailTask.status).text"
              :severity="getTaskStatusConfig(detailTask.status).severity"
              rounded
              class="max-md:hidden"
            />
            <Button
              text
              rounded
              size="small"
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
                {{ getTaskStatusConfig(detailTask.status).text }}
              </span>
            </span>
          </div>
          <div
            v-if="detailTask.status === 'failed'"
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
              <span class="text-admin-caption font-bold text-admin-text-3">账号 ID</span>
              <span class="min-w-0 truncate text-admin-body font-semibold text-admin-text-1">
                {{ detailTask.account_id || 'N/A' }}
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
              class="ml-auto !text-admin-caption !text-admin-text-3"
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
              class="ml-auto !text-admin-caption !text-admin-text-3"
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
import Button from 'primevue/button'
import Chart from 'primevue/chart'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import Dialog from 'primevue/dialog'
import IconField from 'primevue/iconfield'
import PrimeImage from 'primevue/image'
import InputIcon from 'primevue/inputicon'
import InputText from 'primevue/inputtext'
import Paginator, { type PageState } from 'primevue/paginator'
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
  Search,
  Terminal,
  WarningTriangle,
  Xmark,
  XmarkCircle,
} from '@iconoir/vue'
import { useAppToast } from '../../../../shared/composables/useAppToast'
import {
  adminChartPlugins,
  adminChartTheme,
  defineAdminChartData,
  defineAdminChartOptions,
  type AdminChartData,
  type AdminChartOptions,
  withAlpha,
} from '../../shared/lib/adminChart'
import { adminDenseDataTablePt } from '../../shared/lib/adminDataTablePt'
import {
  formatTaskCreatedAt,
  getTaskApiCall,
  getTaskDuration,
  getTaskResultImageUrl,
  getTaskSanitizedError,
  getTaskStatusConfig,
  getTaskUserIdentifier,
  taskStatusOptions,
} from './taskMonitorColumns'
import { useTaskResultDetail } from './useTaskResultDetail'
import { TASK_MONITOR_PAGE_SIZE, useTaskMonitor } from './useTaskMonitor'

type PrimeChartSelectEvent = {
  element?: {
    index?: number
  }
}

const appToast = useAppToast()
const detailDialogPt = {
  root: {
    class:
      '!overflow-hidden !border !border-[rgba(215,224,235,0.92)] !bg-[linear-gradient(180deg,rgba(255,255,255,0.99),rgba(247,250,253,0.96))] !shadow-[0_24px_60px_rgba(45,64,89,0.16)]',
  },
  header: {
    class:
      '!border-b !border-admin-border-soft !px-6 !pt-5 !pb-3.5 max-[720px]:!px-[18px] max-[720px]:!pt-4 max-[720px]:!pb-3',
  },
  content: {
    class:
      '!max-h-[75vh] !overflow-y-auto !px-6 !pt-[18px] !pb-6 max-[720px]:!px-[18px] max-[720px]:!pt-3.5 max-[720px]:!pb-5',
  },
} as const
const taskPaginatorPt = {
  root: {
    class: '!border-0 !bg-transparent !p-0',
  },
  content: {
    class:
      '!flex !h-9 !items-center !gap-1 !rounded-lg !border !border-admin-border !bg-white !p-0.5',
  },
  prev: {
    class:
      '!h-8 !min-w-8 !rounded-md !text-admin-text-2 hover:!bg-admin-surface-muted disabled:!opacity-40',
  },
  next: {
    class:
      '!h-8 !min-w-8 !rounded-md !text-admin-text-2 hover:!bg-admin-surface-muted disabled:!opacity-40',
  },
  current: {
    class:
      '!flex !h-8 !min-w-[72px] !items-center !justify-center !px-2 !text-admin-caption !font-semibold !text-admin-text-2',
  },
} as const

const detailBannerBaseClass =
  'relative flex flex-col gap-3 overflow-hidden rounded-[18px] border border-admin-border-soft bg-admin-surface-muted px-[18px] py-4 before:absolute before:top-3.5 before:bottom-3.5 before:left-0 before:w-1 before:rounded-full before:bg-admin-text-3'
const detailStatusBannerClassByStatus: Record<string, string> = {
  completed:
    'bg-[linear-gradient(135deg,rgba(47,158,115,0.1),rgba(255,255,255,0.74))] before:bg-admin-success',
  failed:
    'bg-[linear-gradient(135deg,rgba(209,77,90,0.095),rgba(255,255,255,0.74))] before:bg-admin-danger',
  running:
    'bg-[linear-gradient(135deg,rgba(212,136,48,0.1),rgba(255,255,255,0.74))] before:bg-admin-warning',
}
const detailStatusDotBaseClass =
  'h-[9px] w-[9px] shrink-0 rounded-full bg-admin-text-3 shadow-[0_0_0_4px_rgba(152,162,179,0.14)]'
const detailStatusDotClassByStatus: Record<string, string> = {
  completed: 'bg-admin-success shadow-[0_0_0_4px_rgba(47,158,115,0.14)]',
  failed: 'bg-admin-danger shadow-[0_0_0_4px_rgba(209,77,90,0.14)]',
  running: 'bg-admin-warning shadow-[0_0_0_4px_rgba(212,136,48,0.15)]',
}

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
} = useTaskResultDetail({ message: appToast })

const {
  tasks,
  total,
  loading,
  page,
  pageStats,
  failedTasks,
  activeTasks,
  slowTasks,
  filterStatus,
  searchKeyword,
  loadTasks,
  handleSearch,
} = useTaskMonitor()

const paginatorFirst = computed(() => Math.max(0, (page.value - 1) * TASK_MONITOR_PAGE_SIZE))
const totalPages = computed(() => Math.ceil(total.value / TASK_MONITOR_PAGE_SIZE) || 1)

watch(totalPages, (newTotal) => {
  if (page.value > newTotal) {
    page.value = Math.max(1, newTotal)
  }
})

function handlePageChange(event: PageState) {
  if (loading.value) return
  page.value = event.page + 1
}
const summaryOtherCount = computed(() =>
  Math.max(
    0,
    tasks.value.length - pageStats.value.running - pageStats.value.pending - pageStats.value.failed,
  ),
)

const longestRunning = computed(() =>
  activeTasks.value.find((row) => row.task.status === 'running'),
)
const longestPending = computed(() =>
  activeTasks.value.find((row) => row.task.status === 'pending'),
)
const currentPageCount = computed(() => tasks.value.length)
const runningFoot = computed(() =>
  longestRunning.value ? `最长 ${formatSeconds(longestRunning.value.seconds)}` : '当前页无执行任务',
)
const pendingFoot = computed(() =>
  longestPending.value ? `最长 ${formatSeconds(longestPending.value.seconds)}` : '当前页无排队任务',
)
const failedFoot = computed(
  () => `当前页失败率 ${formatPercent(pageStats.value.failed, currentPageCount.value)}`,
)
const otherFoot = computed(() => `当前页 ${currentPageCount.value} / 查询 ${total.value}`)

const slowTaskChartData = computed<AdminChartData>(() =>
  defineAdminChartData({
    labels: slowTasks.value.map((row) => row.task.task_id),
    datasets: [
      {
        type: 'bar',
        label: '耗时',
        data: slowTasks.value.map((row) => row.seconds),
        backgroundColor: withAlpha(adminChartTheme.info, 0.72),
        hoverBackgroundColor: adminChartTheme.info,
        borderRadius: 6,
        barThickness: 12,
      },
    ],
  }),
)

const slowTaskChartOptions = computed<AdminChartOptions>(() =>
  defineAdminChartOptions({
    responsive: true,
    maintainAspectRatio: false,
    indexAxis: 'y',
    animation: false,
    plugins: {
      legend: { display: false },
      tooltip: {
        backgroundColor: 'rgba(255, 255, 255, 0.96)',
        titleColor: 'var(--color-admin-text-1)',
        bodyColor: 'var(--color-admin-text-1)',
        bodyFont: { size: 12 },
        titleFont: { size: 12, weight: 700 },
        padding: 10,
        displayColors: false,
        borderWidth: 1,
        borderColor: 'var(--color-admin-border-soft)',
        callbacks: {
          label: (context) => `耗时: ${Number(context.raw || 0)}s`,
        },
      },
    },
    scales: {
      x: {
        beginAtZero: true,
        grid: { color: 'rgba(71, 117, 168, 0.12)' },
        border: { display: false },
        ticks: {
          color: 'var(--color-admin-text-3)',
          callback: (value) => `${Number(value)}s`,
        },
      },
      y: {
        grid: { display: false },
        border: { display: false },
        ticks: {
          color: 'var(--color-admin-text-2)',
          callback: (_value, index) =>
            truncateChartLabel(slowTasks.value[index]?.task.task_id || '', 18),
        },
      },
    },
  }),
)

function handleSlowTaskChartSelect(event: PrimeChartSelectEvent) {
  const index = event.element?.index
  if (typeof index !== 'number') return
  const row = slowTasks.value[index]
  if (!row) return
  showDetail(row.task)
}

function clearSearch() {
  searchKeyword.value = ''
  handleSearch()
}

function getTaskStatusBannerClass(status: string) {
  return [detailBannerBaseClass, detailStatusBannerClassByStatus[status]]
}

function getTaskStatusDotClass(status: string) {
  return [detailStatusDotBaseClass, detailStatusDotClassByStatus[status]]
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

function formatSeconds(seconds: number) {
  if (seconds < 60) return `${seconds}s`
  const minutes = Math.floor(seconds / 60)
  const rest = seconds % 60
  if (minutes < 60) return rest === 0 ? `${minutes}m` : `${minutes}m ${rest}s`
  const hours = Math.floor(minutes / 60)
  const minuteRest = minutes % 60
  return minuteRest === 0 ? `${hours}h` : `${hours}h ${minuteRest}m`
}

function formatPercent(value: number, totalValue: number) {
  if (totalValue <= 0) return '0.0%'
  return `${((value / totalValue) * 100).toFixed(1)}%`
}

function truncateChartLabel(value: string, maxLength: number) {
  if (value.length <= maxLength) return value
  return `${value.slice(0, maxLength - 1)}…`
}
</script>
