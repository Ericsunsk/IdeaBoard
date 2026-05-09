<template>
  <div class="admin-page records-page">
    <div class="summary-grid">
      <AdminSummaryCard label="总任务数" :value="statsTotal" />
      <AdminSummaryCard label="成功" :value="statsSuccess" />
      <AdminSummaryCard label="失败" :value="statsFailed" />
      <AdminSummaryCard label="进行中" :value="statsPending" />
    </div>

    <div class="toolbar">
      <div class="toolbar-left">
        <n-select
          v-model:value="filterStatus"
          :options="statusOptions"
          placeholder="状态筛选"
          clearable
          style="width: 130px"
          @update:value="() => { page = 1; loadRecords() }"
        />
        <n-button secondary class="icon-btn" @click="loadRecords">
          <template #icon><n-icon><RefreshOutline /></n-icon></template>
        </n-button>
      </div>
    </div>

    <div class="table-card admin-table-card">
      <div class="admin-card-header">
        <div class="admin-card-title-wrapper">
          <n-icon size="18" class="admin-card-title-icon"><ListOutline /></n-icon>
          <h3 class="admin-card-title">近期任务记录</h3>
        </div>
      </div>

      <n-data-table
        :columns="columns"
        :data="records"
        :loading="loading"
        :pagination="false"
        :bordered="false"
        :row-key="(row: Task) => row.task_id"
        size="small"
        :scroll-x="1200"
        class="admin-table-dense"
      />
      <div class="pagination-wrapper">
        <n-pagination
          v-model:page="page"
          :page-count="pageCount"
          :page-size="pageSize"
          show-quick-jumper
          @update:page="loadRecords"
        />
      </div>
    </div>

    <!-- 详情抽屉 -->
    <n-drawer v-model:show="drawerVisible" :width="520" placement="right">
      <n-drawer-content :title="'任务详情 - ' + (detailTask?.task_id || '')">
        <div v-if="detailTask" class="detail-content">
          <n-descriptions :column="2" label-placement="left" bordered size="small">
            <n-descriptions-item label="用户ID" :span="1">{{ detailTask.user_id }}</n-descriptions-item>
            <n-descriptions-item label="账号ID" :span="1">{{ detailTask.account_id || '未分配' }}</n-descriptions-item>
            <n-descriptions-item label="状态" :span="1">
              <n-tag :type="getStatusConfig(detailTask.status).type" size="small">
                {{ getStatusConfig(detailTask.status).text }}
              </n-tag>
            </n-descriptions-item>
            <n-descriptions-item label="耗时" :span="1">
              <span v-if="detailTask.completed_at" class="detail-duration is-completed">{{ calcDuration(detailTask) }}</span>
              <span v-else class="detail-duration is-empty">-</span>
            </n-descriptions-item>
            <n-descriptions-item label="创建时间" :span="2">{{ detailTask.created_at }}</n-descriptions-item>
          </n-descriptions>

          <h4 class="detail-subtitle">提示词</h4>
          <div class="detail-block">{{ detailTask.prompt || '-' }}</div>

          <div v-if="detailTask.status === 'failed'">
            <h4 class="detail-subtitle is-danger">失败原因</h4>
            <div class="detail-block error-block">{{ getErrorReason(detailTask) || '-' }}</div>
          </div>

          <div v-if="resultImageUrl" class="detail-image">
            <h4 class="detail-subtitle">生成图片</h4>
            <n-image :src="resultImageUrl" :width="460" object-fit="contain" />
          </div>

          <h4 class="detail-subtitle">API 调用结果摘要</h4>
          <div class="detail-block">
            <pre v-if="resultSummaryText">{{ resultSummaryText }}</pre>
            <span v-else>-</span>
          </div>

          <div v-if="resultDebugText" class="result-debug-toggle">
            <n-button text size="small" @click="showDebugResult = !showDebugResult">
              {{ showDebugResult ? '隐藏排障详情' : '显示排障详情（脱敏）' }}
            </n-button>
          </div>
          <div v-if="showDebugResult && resultDebugText" class="detail-block debug-block">
            <pre>{{ resultDebugText }}</pre>
          </div>
        </div>
      </n-drawer-content>
    </n-drawer>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, h } from 'vue'

import type { Task } from '../../../shared/api/task'
import { adminTaskApi } from '../../../shared/api/adminTask'
import { monitorApi } from '../../../shared/api/monitor'
import {
  NIcon, NTag, NButton, NSelect, NDataTable, NPagination,
  NDrawer, NDrawerContent, NDescriptions, NDescriptionsItem, NImage,
} from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import {
  RefreshOutline,
  EyeOutline,
  ListOutline,
} from '@vicons/ionicons5'
import { sanitizeApiErrorMessage, sanitizeTaskErrorMessage } from '../../../shared/utils/errorMessages'
import AdminSummaryCard from '../shared/components/AdminSummaryCard.vue'

const records = ref<Task[]>([])
const loading = ref(false)
const page = ref(1)
const pageSize = 20
const total = ref(0)
const pageCount = computed(() => Math.ceil(total.value / pageSize))
const filterStatus = ref<string | null>(null)

const statsTotal = ref(0)
const statsSuccess = ref(0)
const statsFailed = ref(0)
const statsPending = ref(0)

const drawerVisible = ref(false)
const detailTask = ref<Task | null>(null)
const showDebugResult = ref(false)
const RESULT_MAX_STRING_LENGTH = 300
const RESULT_DEBUG_MAX_DEPTH = 4
const RESULT_DEBUG_MAX_KEYS = 40
const RESULT_ARRAY_PREVIEW_LENGTH = 5
const SENSITIVE_KEY_PATTERN = /token|secret|password|cookie|authorization|api[-_]?key|signature|credential/i

const resultImageUrl = computed(() => {
  if (!detailTask.value?.result) return ''
  try {
    const r = JSON.parse(detailTask.value.result)
    return r.image_url || ''
  } catch {
    return ''
  }
})

const resultSummaryText = computed(() => {
  if (!detailTask.value?.result) return ''
  const parsed = parseResultSafely(detailTask.value.result)
  if (parsed == null) return sanitizeApiErrorMessage(detailTask.value.result, '[REDACTED]')
  if (Array.isArray(parsed)) {
    return JSON.stringify({ type: 'array', length: parsed.length, preview: sanitizeValue(parsed, 1) }, null, 2)
  }
  if (typeof parsed !== 'object') {
    return truncateString(String(parsed))
  }

  const summary: Record<string, unknown> = {}
  const source = parsed as Record<string, unknown>
  const preferredKeys = [
    'status',
    'platform',
    'model_set',
    'model',
    'request_id',
    'task_id',
    'code',
    'message',
    'error',
    'duration',
    'image_url',
  ]

  preferredKeys.forEach((key) => {
    if (key in source) summary[key] = sanitizeValue(source[key], 0)
  })

  if ('images' in source) {
    const images = Array.isArray(source.images) ? source.images.length : 0
    summary.images = images
  }

  if (Object.keys(summary).length === 0) {
    summary.keys = Object.keys(source).slice(0, 12)
    summary.preview = sanitizeValue(source, 1)
  }

  return JSON.stringify(summary, null, 2)
})

const resultDebugText = computed(() => {
  if (!detailTask.value?.result) return ''
  const parsed = parseResultSafely(detailTask.value.result)
  if (parsed == null) return sanitizeApiErrorMessage(detailTask.value.result, '[REDACTED]')
  return JSON.stringify(sanitizeValue(parsed, 0), null, 2)
})

const statusOptions = [
  { label: '等待中', value: 'pending' },
  { label: '执行中', value: 'running' },
  { label: '已完成', value: 'completed' },
  { label: '失败', value: 'failed' },
]

const getStatusConfig = (status: string) => {
  switch (status) {
    case 'pending': return { type: 'default' as const, text: '等待中' }
    case 'running': return { type: 'warning' as const, text: '执行中' }
    case 'completed': return { type: 'success' as const, text: '已完成' }
    case 'failed': return { type: 'error' as const, text: '失败' }
    default: return { type: 'default' as const, text: status }
  }
}

function parseResultSafely(str: string): unknown | null {
  try {
    return JSON.parse(str)
  } catch {
    return null
  }
}

function truncateString(value: string, maxLength = RESULT_MAX_STRING_LENGTH): string {
  if (value.length <= maxLength) return value
  return `${value.slice(0, maxLength)}...(+${value.length - maxLength} chars)`
}

function sanitizeValue(value: unknown, depth: number, keyCountState = { count: 0 }): unknown {
  if (value == null) return value

  if (typeof value === 'string') {
    const sanitized = sanitizeApiErrorMessage(value, '[REDACTED]')
    return sanitized === '[REDACTED]' ? sanitized : truncateString(sanitized)
  }

  if (typeof value === 'number' || typeof value === 'boolean') {
    return value
  }

  if (Array.isArray(value)) {
    if (depth >= RESULT_DEBUG_MAX_DEPTH) return `[array:${value.length}]`
    const preview = value.slice(0, RESULT_ARRAY_PREVIEW_LENGTH).map((item) => sanitizeValue(item, depth + 1, keyCountState))
    if (value.length > RESULT_ARRAY_PREVIEW_LENGTH) preview.push(`...(+${value.length - RESULT_ARRAY_PREVIEW_LENGTH} items)`)
    return preview
  }

  if (typeof value === 'object') {
    if (depth >= RESULT_DEBUG_MAX_DEPTH) return '[object]'
    const obj = value as Record<string, unknown>
    const entries = Object.entries(obj)
    const result: Record<string, unknown> = {}
    for (const [k, v] of entries) {
      if (keyCountState.count >= RESULT_DEBUG_MAX_KEYS) {
        result.__truncated__ = `...(+${entries.length - Object.keys(result).length} keys)`
        break
      }
      keyCountState.count += 1
      if (SENSITIVE_KEY_PATTERN.test(k)) {
        result[k] = '[REDACTED]'
      } else {
        result[k] = sanitizeValue(v, depth + 1, keyCountState)
      }
    }
    return result
  }

  return String(value)
}

function showDetail(row: Task) {
  detailTask.value = row
  showDebugResult.value = false
  drawerVisible.value = true
}

function getErrorReason(row: Task): string {
  if (row.error_msg) return sanitizeTaskErrorMessage(row.error_msg)
  if (!row.result) return ''
  try {
    const r = JSON.parse(row.result)
    if (r.error) return sanitizeTaskErrorMessage(r.error)
    if (r.status === 'failed' && r.error) return sanitizeTaskErrorMessage(r.error)
  } catch {
    if (row.status === 'failed') return sanitizeTaskErrorMessage(row.result)
  }
  return ''
}

function calcDuration(row: Task): string {
  if (!row.completed_at || !row.created_at) return '-'
  const start = new Date(row.created_at).getTime()
  const end = new Date(row.completed_at).getTime()
  if (isNaN(start) || isNaN(end)) return '-'
  const sec = Math.round((end - start) / 1000)
  return sec >= 0 ? `${sec}s` : '-'
}

const columns: DataTableColumns<Task> = [
  {
    title: '任务ID',
    key: 'task_id',
    width: 180,
    ellipsis: { tooltip: true },
    render: (row) => h('span', { style: { fontFamily: 'monospace', fontSize: '12px' } }, row.task_id),
  },
  {
    title: '用户/账号',
    key: 'user_account',
    width: 100,
    align: 'center',
    render: (row) => h('span', { style: { fontSize: '12px', lineHeight: '1.6' } }, [
      h('span', { style: { color: 'var(--admin-text-3)' } }, 'U:'),
      h('span', { style: { color: 'var(--admin-text-1)', fontWeight: '500' } }, String(row.user_id)),
      h('span', { style: { color: 'var(--admin-text-3)', margin: '0 4px' } }, '/'),
      h('span', { style: { color: 'var(--admin-text-3)' } }, 'A:'),
      h('span', { style: { color: 'var(--admin-text-1)', fontWeight: '500' } }, row.account_id ? String(row.account_id) : '-'),
    ]),
  },
  {
    title: '提示词',
    key: 'prompt',
    width: 200,
    ellipsis: { tooltip: true },
  },
  {
    title: '状态',
    key: 'status',
    width: 80,
    align: 'center',
    render: (row) => {
      const config = getStatusConfig(row.status)
      return h(NTag, { type: config.type, size: 'small' }, () => config.text)
    },
  },
  {
    title: '耗时',
    key: 'duration',
    width: 70,
    align: 'center',
    render: (row) => {
      const dur = calcDuration(row)
      if (dur === '-') return h('span', { style: { color: 'var(--admin-text-3)' } }, '-')
      return h('span', { style: { fontSize: '12px', color: 'var(--admin-success)', fontWeight: '500' } }, dur)
    },
  },
  {
    title: 'API调用',
    key: 'result',
    width: 120,
    render: (row) => {
      if (!row.result) return h('span', { style: { color: 'var(--admin-text-3)' } }, '-')
      try {
        const r = JSON.parse(row.result)
        if (r.platform) {
          return h('span', { style: { fontSize: '12px' } }, [
            h(NTag, { size: 'tiny', type: 'info', bordered: false }, () => r.platform),
            r.model_set ? h('span', { style: { marginLeft: '4px', color: 'var(--admin-text-3)', fontSize: '11px' } }, r.model_set) : null,
          ])
        }
      } catch {}
      return h('span', { style: { color: 'var(--admin-text-2)', fontSize: '12px' } }, '有结果')
    },
  },
  {
    title: '失败原因',
    key: 'error_msg',
    width: 180,
    ellipsis: { tooltip: true },
    render: (row) => {
      const reason = getErrorReason(row)
      return reason
        ? h('span', { style: { color: 'var(--admin-danger)', fontSize: '12px' } }, reason)
        : h('span', { style: { color: 'var(--admin-text-3)' } }, '-')
    },
  },
  {
    title: '创建时间',
    key: 'created_at',
    width: 160,
    render: (row) => h('span', { style: { fontSize: '12px' } }, row.created_at),
  },
  {
    title: '操作',
    key: 'actions',
    width: 70,
    align: 'center',
    render: (row) =>
      h(NButton, { size: 'tiny', quaternary: true, onClick: () => showDetail(row) }, {
        icon: () => h(NIcon, null, () => h(EyeOutline)),
        default: () => '详情',
      }),
  },
]

async function loadRecords() {
  loading.value = true
  try {
    const res = await adminTaskApi.list({
      status: filterStatus.value || undefined,
      page: page.value,
      page_size: pageSize,
    })
    if (res.code === 0) {
      records.value = res.data?.list || []
      total.value = res.data?.total || 0
    }
  } catch {
    console.error('加载绘图记录失败')
  } finally {
    loading.value = false
  }
}

async function loadStats() {
  try {
    const res = await monitorApi.get()
    if (res.code === 0) {
      const s = (res.data as any)?.stats || {}
      statsSuccess.value = s.completed || 0
      statsFailed.value = s.failed || 0
      statsPending.value = (s.pending || 0) + (s.running || 0)
      statsTotal.value = statsSuccess.value + statsFailed.value + statsPending.value
    }
  } catch {}
}

onMounted(() => {
  loadRecords()
  loadStats()
})
</script>

<style scoped>
@import '../shared/adminCommon.css';

.records-page {
  gap: 14px;
}

/* 详情抽屉 */
.detail-content {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.detail-subtitle {
  font-size: 14px;
  font-weight: 600;
  color: var(--admin-text-1);
  margin: 0;
}

.detail-block {
  background: var(--admin-surface-muted);
  border-radius: 14px;
  border: 1px solid var(--admin-border);
  padding: 12px;
  font-size: 13px;
  color: var(--admin-text-2);
  word-break: break-all;
  white-space: pre-wrap;
  max-height: 200px;
  overflow-y: auto;
}

.detail-block pre {
  margin: 0;
  font-family: monospace;
  font-size: 12px;
  white-space: pre-wrap;
}

.result-debug-toggle {
  margin-top: -8px;
}

.detail-duration.is-completed {
  color: var(--admin-success);
  font-weight: 500;
}

.detail-duration.is-empty {
  color: var(--admin-text-3);
}

.detail-subtitle.is-danger {
  color: var(--admin-danger);
}

.debug-block {
  max-height: 260px;
}

.error-block {
  color: var(--admin-danger);
  border: 1px solid rgba(251, 113, 133, 0.28);
}

.detail-image { display: flex; flex-direction: column; gap: 8px; }
</style>
