<template>
  <div class="records-page">
    <!-- 顶部统计卡片 -->
    <div class="stats-grid">
      <div class="stat-card">
        <div class="stat-icon" style="background: #6366f1;">
          <n-icon size="24"><ListOutline /></n-icon>
        </div>
        <div class="stat-content">
          <div class="stat-value">{{ statsTotal }}</div>
          <div class="stat-label">总任务数</div>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon" style="background: #22c55e;">
          <n-icon size="24"><CheckmarkCircleOutline /></n-icon>
        </div>
        <div class="stat-content">
          <div class="stat-value">{{ statsSuccess }}</div>
          <div class="stat-label">成功</div>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon" style="background: #ef4444;">
          <n-icon size="24"><CloseCircleOutline /></n-icon>
        </div>
        <div class="stat-content">
          <div class="stat-value">{{ statsFailed }}</div>
          <div class="stat-label">失败</div>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon" style="background: #f59e0b;">
          <n-icon size="24"><TimeOutline /></n-icon>
        </div>
        <div class="stat-content">
          <div class="stat-value">{{ statsPending }}</div>
          <div class="stat-label">进行中</div>
        </div>
      </div>
    </div>

    <!-- 筛选 & 表格 -->
    <div class="table-section">
      <div class="section-header">
        <h3>绘图记录</h3>
        <n-space>
          <n-select
            v-model:value="filterStatus"
            :options="statusOptions"
            placeholder="状态筛选"
            clearable
            style="width: 130px"
            @update:value="() => { page = 1; loadRecords() }"
          />
          <n-button @click="loadRecords">
            <template #icon><n-icon><RefreshOutline /></n-icon></template>
            刷新
          </n-button>
        </n-space>
      </div>

      <div class="table-card">
        <n-data-table
          :columns="columns"
          :data="records"
          :loading="loading"
          :pagination="false"
          :bordered="false"
          :row-key="(row: Task) => row.task_id"
          size="small"
          :scroll-x="1200"
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
              <span v-if="detailTask.completed_at" style="color: #22c55e; font-weight: 500;">{{ calcDuration(detailTask) }}</span>
              <span v-else style="color: #71717a;">-</span>
            </n-descriptions-item>
            <n-descriptions-item label="创建时间" :span="2">{{ detailTask.created_at }}</n-descriptions-item>
          </n-descriptions>

          <h4 class="detail-subtitle">提示词</h4>
          <div class="detail-block">{{ detailTask.prompt || '-' }}</div>

          <div v-if="detailTask.status === 'failed'">
            <h4 class="detail-subtitle" style="color: #ef4444;">失败原因</h4>
            <div class="detail-block error-block">{{ getErrorReason(detailTask) || '-' }}</div>
          </div>

          <div v-if="resultImageUrl" class="detail-image">
            <h4 class="detail-subtitle">生成图片</h4>
            <n-image :src="resultImageUrl" :width="460" object-fit="contain" />
          </div>

          <h4 class="detail-subtitle">API 调用结果</h4>
          <div class="detail-block">
            <pre v-if="detailTask.result">{{ formatJSON(detailTask.result) }}</pre>
            <span v-else>-</span>
          </div>
        </div>
      </n-drawer-content>
    </n-drawer>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, h } from 'vue'
import accountApi from '../../api/account'
import type { Task } from '../../api/account'
import {
  NIcon, NTag, NButton, NSpace, NSelect, NDataTable, NPagination,
  NDrawer, NDrawerContent, NDescriptions, NDescriptionsItem, NText, NImage,
  NEllipsis,
} from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import {
  ListOutline, CheckmarkCircleOutline, CloseCircleOutline, TimeOutline, RefreshOutline,
  EyeOutline,
} from '@vicons/ionicons5'

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

const resultImageUrl = computed(() => {
  if (!detailTask.value?.result) return ''
  try {
    const r = JSON.parse(detailTask.value.result)
    return r.image_url || ''
  } catch {
    return ''
  }
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

function formatJSON(str: string) {
  try {
    return JSON.stringify(JSON.parse(str), null, 2)
  } catch {
    return str
  }
}

function showDetail(row: Task) {
  detailTask.value = row
  drawerVisible.value = true
}

function getErrorReason(row: Task): string {
  if (row.error_msg) return row.error_msg
  if (!row.result) return ''
  try {
    const r = JSON.parse(row.result)
    if (r.error) return r.error
    if (r.status === 'failed' && r.error) return r.error
  } catch {
    if (row.status === 'failed') return row.result
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
      h('span', { style: { color: '#a1a1aa' } }, 'U:'),
      h('span', { style: { color: '#fafafa', fontWeight: '500' } }, String(row.user_id)),
      h('span', { style: { color: '#3f3f46', margin: '0 4px' } }, '/'),
      h('span', { style: { color: '#a1a1aa' } }, 'A:'),
      h('span', { style: { color: '#fafafa', fontWeight: '500' } }, row.account_id ? String(row.account_id) : '-'),
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
      if (dur === '-') return h('span', { style: { color: '#71717a' } }, '-')
      return h('span', { style: { fontSize: '12px', color: '#22c55e', fontWeight: '500' } }, dur)
    },
  },
  {
    title: 'API调用',
    key: 'result',
    width: 120,
    render: (row) => {
      if (!row.result) return h('span', { style: { color: '#71717a' } }, '-')
      try {
        const r = JSON.parse(row.result)
        if (r.platform) {
          return h('span', { style: { fontSize: '12px' } }, [
            h(NTag, { size: 'tiny', type: 'info', bordered: false }, () => r.platform),
            r.model_set ? h('span', { style: { marginLeft: '4px', color: '#71717a', fontSize: '11px' } }, r.model_set) : null,
          ])
        }
      } catch {}
      return h('span', { style: { color: '#71717a', fontSize: '12px' } }, '有结果')
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
        ? h('span', { style: { color: '#ef4444', fontSize: '12px' } }, reason)
        : h('span', { style: { color: '#71717a' } }, '-')
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
    const res = await accountApi.listAdminTasks({
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
    const res = await accountApi.getMonitor()
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
.records-page {
  padding: 28px 32px;
  display: flex;
  flex-direction: column;
  gap: 20px;
  max-width: 1400px;
}

.stats-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
  gap: 14px;
}

.stat-card {
  background: #18181b;
  border: 1px solid #27272a;
  border-radius: 12px;
  padding: 18px 20px;
  display: flex;
  align-items: center;
  gap: 14px;
}

.stat-icon {
  width: 44px;
  height: 44px;
  border-radius: 10px;
  display: flex;
  align-items: center;
  justify-content: center;
  color: white;
  flex-shrink: 0;
}

.stat-content {
  display: flex;
  flex-direction: column;
}

.stat-value {
  font-size: 24px;
  font-weight: 700;
  color: #fafafa;
  line-height: 1;
}

.stat-label {
  font-size: 13px;
  color: #71717a;
  margin-top: 2px;
}

.table-section {
  background: #18181b;
  border: 1px solid #27272a;
  border-radius: 12px;
  padding: 20px;
}

.section-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 16px;
}

h3 {
  font-size: 16px;
  font-weight: 600;
  color: #fafafa;
  margin: 0;
}

.table-card {
  background: #27272a;
  border-radius: 8px;
  padding: 16px;
}

.pagination-wrapper {
  margin-top: 16px;
  display: flex;
  justify-content: flex-end;
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
  color: #d4d4d8;
  margin: 0;
}

.detail-block {
  background: #27272a;
  border-radius: 6px;
  padding: 12px;
  font-size: 13px;
  color: #a1a1aa;
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

.error-block {
  color: #ef4444;
  border: 1px solid rgba(239, 68, 68, 0.2);
}

.detail-image {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
:deep(.n-data-table) {
  --n-td-color: transparent;
  --n-th-color: transparent;
  --n-border-color: #27272a;
  --n-td-color-hover: rgba(99,102,241,0.05);
  --n-th-text-color: #71717a;
  --n-td-text-color: #e4e4e7;
}
</style>
