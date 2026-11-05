<template>
  <div :class="adminTableCardClass">
    <AdminTable
      :value="tasks"
      :loading="loading"
      data-key="task_id"
      scrollable
      :frame-class="adminTableFrameClass"
      :table-style="adminTableStyle"
      :class="adminTableClass"
    >
      <template #empty>
        <AdminEmptyState :title="MONITOR_MESSAGES.tableEmpty" />
      </template>

      <!-- 第 1 列：Service (service 图标 + slug + 请求清晰度) -->
      <AdminColumn
        :header="MONITOR_MESSAGES.serviceColumn"
        :style="{ width: adminMonitorColumnWidths.service }"
      >
        <template #body="{ data }">
          <AdminMono as="div" class="flex items-center gap-1.5 text-xs text-foreground">
            <component
              :is="getTaskRowView(data).serviceIcon"
              v-if="getTaskRowView(data).serviceIcon"
              class="size-4 shrink-0 text-muted-foreground"
            />
            <FastArrowRight v-else class="size-3.5 text-warning shrink-0" />
            <span
              class="truncate text-foreground"
              :title="emptyPlaceholder(getTaskRowView(data).serviceSlug)"
              >{{ emptyPlaceholder(getTaskRowView(data).serviceSlug) }}</span
            >
            <span
              v-if="getTaskServiceResolution(data)"
              class="shrink-0 rounded-control border border-border bg-muted px-1 font-mono text-admin-caption text-muted-foreground"
              :title="getTaskServiceResolution(data)"
            >
              {{ getTaskServiceResolution(data) }}
            </span>
          </AdminMono>
        </template>
      </AdminColumn>

      <!-- 模型列：实际解析模型 -->
      <AdminColumn
        :header="MONITOR_MESSAGES.modelColumn"
        :style="{ width: adminMonitorColumnWidths.model }"
      >
        <template #body="{ data }">
          <AdminMono
            class="truncate text-xs text-foreground"
            :title="emptyPlaceholder(getTaskModelLabel(data))"
          >
            {{ emptyPlaceholder(getTaskModelLabel(data)) }}
          </AdminMono>
        </template>
      </AdminColumn>

      <!-- 第 3 列：Status (复用 Events 表状态 Token 胶囊与点) -->
      <AdminColumn
        :header="MONITOR_MESSAGES.statusColumn"
        :style="{ width: adminMonitorColumnWidths.status }"
      >
        <template #body="{ data }">
          <TaskStatusCell :row="data" />
        </template>
      </AdminColumn>

      <!-- 结果列：最多 3 张缩略图，超出 +N，点击进详情 -->
      <AdminColumn
        :header="MONITOR_MESSAGES.resultColumn"
        :style="{ width: adminMonitorColumnWidths.result }"
      >
        <template #body="{ data }">
          <TaskResultCell :row="data" @inspect="emit('showDetail', $event)" />
        </template>
      </AdminColumn>

      <!-- 第 5 列：Message (事件 Prompt / 诊断内容，服务名前缀已由 Service 列承载) -->
      <AdminColumn
        :header="MONITOR_MESSAGES.messageColumn"
        :style="{ width: adminMonitorColumnWidths.message }"
      >
        <template #body="{ data }">
          <AdminMono
            as="div"
            class="max-w-[160px] truncate text-xs text-foreground"
            :title="emptyPlaceholder(data.prompt)"
          >
            {{ emptyPlaceholder(getTaskRowView(data).message) }}
          </AdminMono>
        </template>
      </AdminColumn>

      <!-- 花费列：实际扣账 Idea 数 -->
      <AdminColumn
        :header="MONITOR_MESSAGES.creditsColumn"
        :style="{ width: adminMonitorColumnWidths.credits }"
      >
        <template #body="{ data }">
          <AdminMono class="text-xs whitespace-nowrap text-foreground tabular-nums">
            {{ data.credits || 0 }} <span class="text-muted-foreground">Idea</span>
          </AdminMono>
        </template>
      </AdminColumn>

      <!-- 第 7 列：时间戳 GMT+8 ↓ [✏ 字段] -->
      <AdminColumn :style="{ width: adminMonitorColumnWidths.timestamp }">
        <template #header>
          <div class="flex items-center gap-1.5 font-sans">
            <span class="font-semibold text-foreground text-xs">{{
              MONITOR_MESSAGES.timestampColumn
            }}</span>
            <span class="font-mono text-admin-caption text-muted-foreground font-normal"
              >GMT+8</span
            >
          </div>
        </template>
        <template #body="{ data }">
          <AdminMono as="div" class="flex items-center gap-2 text-xs tabular-nums">
            <!-- 竖直状态指示条（状态 Pill 语义 dot 色，与 Status 列同源；未知状态回退中性 token，不用 raw 调色板） -->
            <span
              :class="[
                'h-3.5 w-[3px] shrink-0 rounded-pill',
                getTaskStatusDefinition(data.status).dotClass || 'bg-muted-foreground',
              ]"
            />
            <!-- 精确时间戳 2026-09-17 12:18:35 -->
            <span class="font-medium text-foreground">
              {{ formatTaskExactTimestamp(data) }}
            </span>
          </AdminMono>
        </template>
      </AdminColumn>

      <!-- 耗时列：完成时间 - 创建时间，未完成展示占位 -->
      <AdminColumn
        :header="MONITOR_MESSAGES.durationColumn"
        :style="{ width: adminMonitorColumnWidths.duration }"
      >
        <template #body="{ data }">
          <AdminMono class="text-xs whitespace-nowrap text-foreground tabular-nums">
            {{ formatTaskDuration(data) }}
          </AdminMono>
        </template>
      </AdminColumn>

      <!-- 第 9 列：操作 (仅取消活跃任务) -->
      <AdminColumn :style="{ width: adminMonitorColumnWidths.actions }">
        <template #body="{ data }">
          <div class="flex items-center justify-end gap-1">
            <AdminButton
              v-if="canCancelTask(data)"
              variant="ghost"
              size="sm"
              severity="warn"
              :class="adminTextButtonClass"
              :label="MONITOR_MESSAGES.cancelAction"
              :is-loading="isCancellingTask(data.task_id)"
              :aria-label="MONITOR_MESSAGES.cancelTaskAria"
              @click="emit('cancelTask', data)"
            />
          </div>
        </template>
      </AdminColumn>

      <!-- 第 10 列：More (更多图标，点击进详情弹窗；演示模式用本地数据预览) -->
      <AdminColumn
        :header="MONITOR_MESSAGES.moreColumn"
        :style="{ width: adminMonitorColumnWidths.more }"
      >
        <template #body="{ data }">
          <div class="flex items-center justify-start">
            <button
              type="button"
              :class="adminTableRowActionButtonClass"
              :aria-label="MONITOR_MESSAGES.moreDetailAria"
              @click="emit('showDetail', data)"
            >
              <MoreHoriz class="size-4" />
            </button>
          </div>
        </template>
      </AdminColumn>
    </AdminTable>

    <!-- 表格底栏 (Card Footer)：灰底带上边框，左侧记录总数，右侧分页控制器 -->
    <div :class="adminTableFooterClass">
      <span class="text-xs font-normal text-muted-foreground">
        {{ MONITOR_MESSAGES.totalRecords(totalCount) }}
      </span>

      <div class="flex items-center gap-2">
        <AdminPagerControls
          :current-page="currentPage"
          :total-pages="totalPages"
          :prev-disabled="!canPrevious"
          :next-disabled="!canNext"
          @prev="emit('prevPage')"
          @next="emit('nextPage')"
        />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { FastArrowRight, MoreHoriz } from '@iconoir/vue'
import { AdminButton } from '../../../shared'
import { AdminColumn } from '../../../shared'
import { AdminEmptyState } from '../../../shared'
import { AdminMono } from '../../../shared'
import { AdminPagerControls } from '../../../shared'
import { AdminTable } from '../../../shared'
import {
  adminTableCardClass,
  adminTableClass,
  adminTableFooterClass,
  adminTableFrameClass,
  adminTableRowActionButtonClass,
  adminTableStyle,
  adminTextButtonClass,
} from '../../../shared'
import { emptyPlaceholder } from '../../../shared'
import TaskResultCell from '../TaskResultCell.vue'
import TaskStatusCell from '../TaskStatusCell.vue'
import { MONITOR_MESSAGES } from '../pages/messages'
import {
  adminMonitorColumnWidths,
  formatTaskExactTimestamp,
  getTaskModelLabel,
  getTaskRowView,
  getTaskServiceResolution,
} from '../columns'
import { getTaskStatusDefinition } from '../../core/status'
import { formatTaskDuration } from '../../core/taskResultPresentation'
import type { AdminTaskRow } from '../../session/useAdminTaskList'

defineProps<{
  tasks: AdminTaskRow[]
  loading: boolean
  totalCount: number
  currentPage: number
  totalPages: number
  canPrevious: boolean
  canNext: boolean
  canCancelTask: (row: AdminTaskRow) => boolean
  isCancellingTask: (taskId: string) => boolean
}>()

const emit = defineEmits<{
  cancelTask: [row: AdminTaskRow]
  showDetail: [row: AdminTaskRow]
  prevPage: []
  nextPage: []
}>()
</script>
