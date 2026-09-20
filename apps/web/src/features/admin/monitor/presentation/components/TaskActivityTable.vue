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
        <AdminEmptyState title="暂无事件日志" />
      </template>

      <!-- 第 1 列：时间戳 GMT+8 ↓ [✏ 字段] -->
      <AdminColumn :style="{ width: adminMonitorColumnWidths.timestamp }">
        <template #header>
          <div class="flex items-center gap-1.5 font-sans">
            <span class="font-semibold text-admin-text-2 text-xs">时间戳</span>
            <span class="font-mono text-[11px] text-admin-text-3 font-normal">GMT+8</span>
          </div>
        </template>
        <template #body="{ data }">
          <div class="flex items-center gap-2 font-mono text-xs">
            <!-- 竖直状态指示条 (复用 Events 表的状态 Token 颜色) -->
            <span
              :class="[
                'h-3.5 w-[3px] shrink-0 rounded-full',
                getTaskRowView(data).metric?.dotClass || 'bg-slate-400',
              ]"
            />
            <!-- 精确时间戳 2026-09-17 12:18:35 -->
            <span class="font-medium text-admin-text-1">
              {{ formatTaskExactTimestamp(data) }}
            </span>
          </div>
        </template>
      </AdminColumn>

      <!-- 耗时列：完成时间 - 创建时间，未完成展示占位 -->
      <AdminColumn header="Duration" :style="{ width: adminMonitorColumnWidths.duration }">
        <template #body="{ data }">
          <span class="font-mono text-xs text-admin-text-1">
            {{ formatTaskDuration(data) }}
          </span>
        </template>
      </AdminColumn>

      <!-- 第 2 列：Status (复用 Events 表状态 Token 胶囊与点) -->
      <AdminColumn header="Status" :style="{ width: adminMonitorColumnWidths.status }">
        <template #body="{ data }">
          <TaskStatusCell :row="data" />
        </template>
      </AdminColumn>

      <!-- 第 3 列：Service (feature 图标 + slug) -->
      <AdminColumn header="Service" :style="{ width: adminMonitorColumnWidths.service }">
        <template #body="{ data }">
          <div class="flex items-center gap-1.5 font-mono text-xs text-admin-text-1">
            <component
              :is="getTaskRowView(data).featureIcon"
              v-if="getTaskRowView(data).featureIcon"
              class="size-4 shrink-0 text-admin-text-2"
            />
            <FastArrowRight v-else class="size-3.5 text-orange-500 shrink-0" />
            <span class="truncate text-slate-700">{{ getTaskRowView(data).featureSlug || '-' }}</span>
          </div>
        </template>
      </AdminColumn>

      <!-- 模型列：实际解析模型 -->
      <AdminColumn header="Model" :style="{ width: adminMonitorColumnWidths.model }">
        <template #body="{ data }">
          <span class="truncate font-mono text-xs text-admin-text-1" :title="getTaskModelLabel(data) || '-'">
            {{ getTaskModelLabel(data) || '-' }}
          </span>
        </template>
      </AdminColumn>

      <!-- 第 4 列：Message (事件 Prompt / 诊断内容，服务名前缀已由 Service 列承载) -->
      <AdminColumn header="Message" :style="{ width: adminMonitorColumnWidths.message }">
        <template #body="{ data }">
          <div class="max-w-[160px] truncate font-mono text-xs text-slate-800" :title="data.prompt || '-'">
            {{ getTaskRowView(data).message || '-' }}
          </div>
        </template>
      </AdminColumn>

      <!-- 花费列：实际扣账 Idea 数 -->
      <AdminColumn header="Credits" :style="{ width: adminMonitorColumnWidths.credits }">
        <template #body="{ data }">
          <span class="font-mono text-xs text-admin-text-1">
            {{ data.credits || 0 }} <span class="text-admin-text-3">Idea</span>
          </span>
        </template>
      </AdminColumn>

      <!-- 结果列：最多 3 张缩略图，超出 +N，点击进详情 -->
      <AdminColumn header="Result" :style="{ width: adminMonitorColumnWidths.result }">
        <template #body="{ data }">
          <TaskResultCell :row="data" @inspect="emit('showDetail', $event)" />
        </template>
      </AdminColumn>

      <!-- 第 7 列：操作 (仅取消活跃任务) -->
      <AdminColumn :style="{ width: adminMonitorColumnWidths.actions }">
        <template #body="{ data }">
          <div class="flex items-center justify-end gap-1">
            <AdminButton
              v-if="canCancelTask(data)"
              variant="ghost"
              size="sm"
              severity="warn"
              :class="adminTextButtonClass"
              label="取消"
              :loading="isCancellingTask(data.task_id)"
              aria-label="取消任务"
              @click="emit('cancelTask', data)"
            />
          </div>
        </template>
      </AdminColumn>

      <!-- 第 8 列：More (更多图标，点击进详情弹窗；演示模式用本地数据预览) -->
      <AdminColumn header="More" :style="{ width: adminMonitorColumnWidths.more }">
        <template #body="{ data }">
          <div class="flex items-center justify-start">
            <button
              type="button"
              class="flex size-8 shrink-0 cursor-pointer items-center justify-center rounded-lg border border-admin-border-soft bg-white text-admin-text-2 transition-colors hover:bg-admin-surface-muted"
              aria-label="更多任务详情"
              @click="emit('showDetail', data)"
            >
              <MoreHoriz class="size-4" />
            </button>
          </div>
        </template>
      </AdminColumn>
    </AdminTable>

    <!-- 表格底栏 (Card Footer)：灰底带上边框，左侧记录总数，右侧分页控制器 -->
    <div class="flex items-center justify-between border-t border-admin-border-soft bg-admin-bg px-4 py-2.5">
      <span class="text-xs font-normal text-admin-text-3">
        共 {{ totalCount }} 条记录
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
import AdminButton from '../../../shared/components/AdminButton.vue'
import AdminColumn from '../../../shared/components/AdminColumn.vue'
import AdminEmptyState from '../../../shared/components/AdminEmptyState.vue'
import AdminPagerControls from '../../../shared/components/AdminPagerControls.vue'
import AdminTable from '../../../shared/components/AdminTable.vue'
import {
  adminTableCardClass,
  adminTableClass,
  adminTableFrameClass,
  adminTableStyle,
  adminTextButtonClass,
} from '../../../shared/lib/uiClasses'
import TaskResultCell from '../TaskResultCell.vue'
import TaskStatusCell from '../TaskStatusCell.vue'
import {
  adminMonitorColumnWidths,
  formatTaskExactTimestamp,
  getTaskModelLabel,
  getTaskRowView,
} from '../columns'
import { formatTaskDuration } from '../../domain/taskResultPresentation'
import type { AdminTaskRow } from '../../application/useAdminTaskList'

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
