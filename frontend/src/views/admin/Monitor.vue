<template>
  <div class="admin-page monitor-page">
    <div class="page-header">
      <div>
        <h2 class="page-title">任务监控中心</h2>
        <p class="page-desc">实时观察队列压力、账号负载与任务运行状态，默认每 5 秒刷新一次监控指标。</p>
      </div>
      <div class="header-side-note">
        <span>Auto Refresh 5s</span>
      </div>
    </div>

    <div class="summary-grid">
      <div class="summary-card">
        <span class="summary-label">等待中</span>
        <span class="summary-value">{{ stats.pending }}</span>
        <span class="summary-foot">待调度任务</span>
      </div>
      <div class="summary-card">
        <span class="summary-label">执行中</span>
        <span class="summary-value">{{ stats.running }}</span>
        <span class="summary-foot">活动任务</span>
      </div>
      <div class="summary-card">
        <span class="summary-label">已完成</span>
        <span class="summary-value">{{ stats.completed }}</span>
        <span class="summary-foot">累计成功</span>
      </div>
      <div class="summary-card">
        <span class="summary-label">失败</span>
        <span class="summary-value">{{ stats.failed }}</span>
        <span class="summary-foot">建议排查上游</span>
      </div>
    </div>

    <div class="queue-section">
      <div class="panel-header">
        <div>
          <h3 class="panel-title">队列状态</h3>
          <p class="panel-desc">用于判断系统是否接近吞吐上限。</p>
        </div>
      </div>
      <div class="queue-grid">
        <div class="queue-card">
          <div class="queue-header">
            <div class="queue-title-wrap">
              <n-icon size="16"><ListOutline /></n-icon>
              <span class="queue-title">全局队列</span>
            </div>
            <n-tag :type="monitor.global_queue_len > 1000 ? 'error' : 'success'" size="small">
              {{ monitor.global_queue_len }}
            </n-tag>
          </div>
          <n-progress
            type="line"
            :percentage="queueUsageRate"
            :show-indicator="false"
            :height="8"
            :border-radius="4"
          />
          <div class="queue-info">
            <span>最大容量: 10000</span>
            <span>{{ queueUsageRate.toFixed(1) }}%</span>
          </div>
        </div>
        <div class="queue-card">
          <div class="queue-header">
            <div class="queue-title-wrap">
              <n-icon size="16"><PlayCircleOutline /></n-icon>
              <span class="queue-title">运行中任务</span>
            </div>
            <n-tag type="warning" size="small">{{ monitor.running_tasks }}</n-tag>
          </div>
          <n-progress
            type="line"
            :percentage="runningPressureRate"
            :show-indicator="false"
            :height="8"
            :border-radius="4"
            status="warning"
          />
        </div>
      </div>
    </div>

    <div class="load-section">
      <div class="panel-header">
        <div>
          <h3 class="panel-title">账号负载</h3>
          <p class="panel-desc">实时展示每个账号的并发占用。</p>
        </div>
      </div>
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
        <div>
          <h3 class="panel-title">最近任务</h3>
          <p class="panel-desc">支持状态筛选，便于快速定位异常任务。</p>
        </div>
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
import { ref, onMounted, onUnmounted, h, computed } from 'vue'
import accountApi from '../../api/account'
import type { Task } from '../../api/account'
import { NIcon, NTag, NProgress, NButton, NSpace, NSelect, NDataTable, NPagination } from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import { ListOutline, PlayCircleOutline, RefreshOutline } from '@vicons/ionicons5'

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
const queueUsageRate = computed(() => Math.min(100, (monitor.value.global_queue_len / 10000) * 100))
const runningPressureRate = computed(() => Math.min(100, (monitor.value.running_tasks / 200) * 100))

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
@import './admin-common.css';

.monitor-page {
  gap: 14px;
}

.queue-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 16px;
}

.queue-card {
  background: rgba(255, 255, 255, 0.78);
  border: 1px solid rgba(136, 151, 208, 0.3);
  border-radius: 16px;
  padding: 12px;
}

.queue-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 12px;
}

.queue-title-wrap {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}

.queue-title {
  font-size: 13px;
  font-weight: 500;
  color: #3a456f;
}

.queue-info {
  display: flex;
  justify-content: space-between;
  margin-top: 8px;
  font-size: 11px;
  color: #8692b9;
}

.load-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
  gap: 16px;
}

.load-card {
  background: rgba(255, 255, 255, 0.78);
  border: 1px solid rgba(136, 151, 208, 0.3);
  border-radius: 16px;
  padding: 12px;
}

.load-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 12px;
}

.load-title {
  font-size: 13px;
  font-weight: 500;
  color: #3a456f;
}

.load-info {
  display: flex;
  justify-content: space-between;
  margin-top: 8px;
  font-size: 11px;
  color: #8692b9;
}

@media (max-width: 920px) {
  .queue-grid {
    grid-template-columns: 1fr;
  }
}
</style>
