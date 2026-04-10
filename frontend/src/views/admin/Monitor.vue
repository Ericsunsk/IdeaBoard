<template>
  <div class="monitor-page">
    <div class="stats-grid">
      <div class="stat-card">
        <div class="stat-icon" style="background: #6366f1;">
          <n-icon size="24"><ListOutline /></n-icon>
        </div>
        <div class="stat-content">
          <div class="stat-value">{{ stats.pending }}</div>
          <div class="stat-label">等待中</div>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon" style="background: #f59e0b;">
          <n-icon size="24"><PlayCircleOutline /></n-icon>
        </div>
        <div class="stat-content">
          <div class="stat-value">{{ stats.running }}</div>
          <div class="stat-label">执行中</div>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon" style="background: #22c55e;">
          <n-icon size="24"><CheckmarkCircleOutline /></n-icon>
        </div>
        <div class="stat-content">
          <div class="stat-value">{{ stats.completed }}</div>
          <div class="stat-label">已完成</div>
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-icon" style="background: #ef4444;">
          <n-icon size="24"><CloseCircleOutline /></n-icon>
        </div>
        <div class="stat-content">
          <div class="stat-value">{{ stats.failed }}</div>
          <div class="stat-label">失败</div>
        </div>
      </div>
    </div>

    <div class="queue-section">
      <h3>队列状态</h3>
      <div class="queue-grid">
        <div class="queue-card">
          <div class="queue-header">
            <span class="queue-title">全局队列</span>
            <n-tag :type="monitor.global_queue_len > 1000 ? 'error' : 'success'" size="small">
              {{ monitor.global_queue_len }}
            </n-tag>
          </div>
          <n-progress
            type="line"
            :percentage="Math.min(100, (monitor.global_queue_len / 10000) * 100)"
            :show-indicator="false"
            :height="8"
            :border-radius="4"
          />
          <div class="queue-info">
            <span>最大容量: 10000</span>
            <span>{{ ((monitor.global_queue_len / 10000) * 100).toFixed(1) }}%</span>
          </div>
        </div>
        <div class="queue-card">
          <div class="queue-header">
            <span class="queue-title">运行中任务</span>
            <n-tag type="warning" size="small">{{ monitor.running_tasks }}</n-tag>
          </div>
          <n-progress
            type="line"
            :percentage="50"
            :show-indicator="false"
            :height="8"
            :border-radius="4"
            status="warning"
          />
        </div>
      </div>
    </div>

    <div class="load-section">
      <h3>账号负载</h3>
      <div class="load-grid">
        <div v-for="(load, accountId) in monitor.account_loads" :key="accountId" class="load-card">
          <div class="load-header">
            <span class="load-title">账号 #{{ accountId }}</span>
            <n-tag :type="load.current_concurrent >= load.max_concurrent ? 'error' : 'success'" size="small">
              {{ load.current_concurrent }}/{{ load.max_concurrent }}
            </n-tag>
          </div>
          <n-progress
            type="line"
            :percentage="(load.current_concurrent / load.max_concurrent) * 100"
            :height="12"
            :border-radius="6"
            :color="load.current_concurrent >= load.max_concurrent ? '#ef4444' : '#6366f1'"
          />
          <div class="load-info">
            <span>当前: {{ load.current_concurrent }}</span>
            <span>最大: {{ load.max_concurrent }}</span>
          </div>
        </div>
        <div v-if="Object.keys(monitor.account_loads).length === 0" class="empty-state">
          暂无账号负载数据
        </div>
      </div>
    </div>

    <div class="tasks-section">
      <div class="section-header">
        <h3>最近任务</h3>
        <n-space>
          <n-select v-model:value="filterStatus" :options="statusOptions" placeholder="筛选状态" clearable style="width: 120px" />
          <n-button @click="loadTasks">
            <template #icon><n-icon><RefreshOutline /></n-icon></template>
            刷新
          </n-button>
        </n-space>
      </div>
      <div class="table-card">
        <n-data-table
          :columns="columns"
          :data="tasks"
          :loading="loading"
          :pagination="false"
          :bordered="false"
          size="small"
        />
        <div class="pagination-wrapper">
          <n-pagination
            v-model:page="page"
            :page-count="pageCount"
            @update:page="loadTasks"
          />
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted, h } from 'vue'
import accountApi from '../../api/account'
import type { Task } from '../../api/account'
import { NIcon, NTag, NProgress, NButton, NSpace, NSelect, NDataTable, NPagination } from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import { ListOutline, PlayCircleOutline, CheckmarkCircleOutline, CloseCircleOutline, RefreshOutline } from '@vicons/ionicons5'

const stats = ref({
  pending: 0,
  running: 0,
  completed: 0,
  failed: 0,
})

const monitor = ref({
  global_queue_len: 0,
  wait_queue_len: 0,
  running_tasks: 0,
  account_loads: {} as Record<number, { current_concurrent: number; max_concurrent: number }>,
})

const tasks = ref<Task[]>([])
const loading = ref(false)
const page = ref(1)
const pageCount = ref(1)
const filterStatus = ref<string | null>(null)

const statusOptions = [
  { label: '等待中', value: 'pending' },
  { label: '执行中', value: 'running' },
  { label: '已完成', value: 'completed' },
  { label: '失败', value: 'failed' },
]

const getStatusConfig = (status: string) => {
  switch (status) {
    case 'pending': return { type: 'default' as const, text: '等待中', color: '#71717a' }
    case 'running': return { type: 'warning' as const, text: '执行中', color: '#f59e0b' }
    case 'completed': return { type: 'success' as const, text: '已完成', color: '#22c55e' }
    case 'failed': return { type: 'error' as const, text: '失败', color: '#ef4444' }
    default: return { type: 'default' as const, text: status, color: '#71717a' }
  }
}

const columns: DataTableColumns<Task> = [
  { title: '任务ID', key: 'task_id', width: 200,
    render: (row) => h('span', { style: { fontFamily: 'monospace', fontSize: '12px' } }, row.task_id) },
  { title: '账号', key: 'account_id', width: 80, align: 'center' },
  { title: '状态', key: 'status', width: 100,
    render: (row) => {
      const config = getStatusConfig(row.status)
      return h(NTag, { type: config.type, size: 'small' }, () => config.text)
    } },
  { title: '重试', key: 'retry_count', width: 60, align: 'center' },
  { title: '创建时间', key: 'created_at', width: 170 },
  { title: '错误信息', key: 'error_msg', ellipsis: { tooltip: true },
    render: (row) => row.error_msg ? h('span', { style: { color: '#ef4444', fontSize: '12px' } }, row.error_msg) : '-' },
]

let refreshTimer: number | null = null

async function loadMonitor() {
  try {
    const res = await accountApi.getMonitor()
    if (res.code === 0) {
      monitor.value = res.data?.monitor || monitor.value
      stats.value = res.data?.stats || stats.value
    }
  } catch (e) {
    console.error('加载监控数据失败', e)
  }
}

async function loadTasks() {
  loading.value = true
  try {
    const res = await accountApi.listAdminTasks({
      status: filterStatus.value || undefined,
      page: page.value,
      page_size: 20,
    })
    if (res.code === 0) {
      tasks.value = res.data?.list || []
      pageCount.value = Math.ceil((res.data?.total || 0) / 20)
    }
  } catch {
    console.error('加载任务列表失败')
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  loadMonitor()
  loadTasks()
  refreshTimer = window.setInterval(() => {
    loadMonitor()
  }, 5000)
})

onUnmounted(() => {
  if (refreshTimer) {
    clearInterval(refreshTimer)
  }
})
</script>

<style scoped>
.monitor-page {
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

.queue-section, .load-section, .tasks-section {
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
  margin: 0 0 16px 0;
}

.queue-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 16px;
}

.queue-card {
  background: #27272a;
  border-radius: 8px;
  padding: 16px;
}

.queue-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 12px;
}

.queue-title {
  font-size: 14px;
  font-weight: 500;
  color: #fafafa;
}

.queue-info {
  display: flex;
  justify-content: space-between;
  margin-top: 8px;
  font-size: 12px;
  color: #71717a;
}

.load-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
  gap: 16px;
}

.load-card {
  background: #27272a;
  border-radius: 8px;
  padding: 16px;
}

.load-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 12px;
}

.load-title {
  font-size: 14px;
  font-weight: 500;
  color: #fafafa;
}

.load-info {
  display: flex;
  justify-content: space-between;
  margin-top: 8px;
  font-size: 12px;
  color: #71717a;
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

.empty-state {
  text-align: center;
  color: #71717a;
  padding: 32px;
  grid-column: 1 / -1;
}
</style>
