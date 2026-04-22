<template>
  <div class="admin-page monitor-page">
    <div class="page-header">
      <div>
        <h2 class="page-title">任务监控中心</h2>
        <p class="page-desc">实时监控任务与账号负载。</p>
      </div>
    </div>

    <div class="summary-grid">
      <AdminSummaryCard label="等待中" :value="stats.pending" />
      <AdminSummaryCard label="执行中" :value="stats.running" />
      <AdminSummaryCard label="已完成" :value="stats.completed" />
      <AdminSummaryCard label="失败" :value="stats.failed" />
    </div>

    <div class="queue-section">
      <AdminSectionHeader title="队列状态" />
      <div class="queue-grid">
        <div class="queue-card is-primary">
          <div class="queue-header">
            <div class="queue-title-wrap">
              <n-icon size="16"><ListOutline /></n-icon>
              <span class="queue-title">全局队列</span>
            </div>
            <n-tag :type="monitor.global_queue_len > GLOBAL_QUEUE_ALERT_THRESHOLD ? 'error' : 'success'" size="small">
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
            <span>最大容量: {{ GLOBAL_QUEUE_CAPACITY }}</span>
            <span>{{ queueUsageRate.toFixed(1) }}%</span>
          </div>
        </div>
        <div class="queue-card is-warning">
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
          <div class="queue-info">
            <span>压力阈值: {{ RUNNING_TASK_PRESSURE_THRESHOLD }}</span>
            <span>{{ runningPressureRate.toFixed(1) }}%</span>
          </div>
        </div>
      </div>
    </div>

    <div class="load-section">
      <AdminSectionHeader title="账号负载" />
      <div class="load-grid">
        <div
          v-for="(load, accountId) in monitor.account_loads"
          :key="accountId"
          class="load-card"
          :class="{ saturated: isAccountSaturated(load) }"
        >
          <div class="load-header">
            <span class="load-title">账号 #{{ accountId }}</span>
            <n-tag :type="isAccountSaturated(load) ? 'error' : 'success'" size="small">
              {{ load.current_concurrent }}/{{ load.max_concurrent }}
            </n-tag>
          </div>
          <n-progress
            type="line"
            :percentage="calculateAccountLoadRate(load)"
            :height="12"
            :border-radius="6"
            :color="isAccountSaturated(load) ? ERROR_TEXT_COLOR : '#6366f1'"
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
      <AdminSectionHeader title="最近任务" variant="section">
        <n-space>
          <n-select v-model:value="filterStatus" :options="taskStatusOptions" placeholder="筛选状态" clearable style="width: 120px" />
          <n-button @click="loadTasks">
            <template #icon><n-icon><RefreshOutline /></n-icon></template>
            刷新
          </n-button>
        </n-space>
      </AdminSectionHeader>
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
import { ref, onMounted, onUnmounted, computed, watch } from 'vue'
import accountApi from '../../api/account'
import type { Task } from '../../api/account'
import { NIcon, NTag, NProgress, NButton, NSpace, NSelect, NDataTable, NPagination } from 'naive-ui'
import { ListOutline, PlayCircleOutline, RefreshOutline } from '@vicons/ionicons5'
import {
  TASK_PAGE_SIZE,
  MONITOR_REFRESH_INTERVAL_MS,
  GLOBAL_QUEUE_CAPACITY,
  RUNNING_TASK_PRESSURE_THRESHOLD,
  ERROR_TEXT_COLOR,
  emptyMonitorStats,
  emptyMonitorState,
  taskStatusOptions,
  GLOBAL_QUEUE_ALERT_THRESHOLD,
  calculateCappedRate,
  calculateAccountLoadRate,
  isAccountSaturated,
  createMonitorTaskColumns,
} from './monitorMeta'
import AdminSummaryCard from './components/AdminSummaryCard.vue'
import AdminSectionHeader from './components/AdminSectionHeader.vue'

const stats = ref({ ...emptyMonitorStats })

const monitor = ref({ ...emptyMonitorState })

const tasks = ref<Task[]>([])
const loading = ref(false)
const page = ref(1)
const pageCount = ref(1)
const filterStatus = ref<string | null>(null)
const queueUsageRate = computed(() => calculateCappedRate(monitor.value.global_queue_len, GLOBAL_QUEUE_CAPACITY))
const runningPressureRate = computed(() =>
  calculateCappedRate(monitor.value.running_tasks, RUNNING_TASK_PRESSURE_THRESHOLD)
)
const columns = createMonitorTaskColumns()

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
      page_size: TASK_PAGE_SIZE,
    })
    if (res.code === 0) {
      tasks.value = res.data?.list || []
      pageCount.value = Math.max(1, Math.ceil((res.data?.total || 0) / TASK_PAGE_SIZE))
    }
  } catch {
    console.error('加载任务列表失败')
  } finally {
    loading.value = false
  }
}

watch(filterStatus, () => {
  page.value = 1
  loadTasks()
})

onMounted(() => {
  loadMonitor()
  loadTasks()
  refreshTimer = window.setInterval(() => {
    loadMonitor()
  }, MONITOR_REFRESH_INTERVAL_MS)
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
  background: #141416;
  border: 1px solid #232326;
  border-radius: 14px;
  padding: 12px;
  transition: border-color 0.18s ease, background 0.18s ease;
}

.queue-card.is-primary {
  border-color: #2d2f33;
}

.queue-card.is-warning {
  border-color: rgba(245, 158, 11, 0.2);
}

.queue-card:hover,
.load-card:hover {
  border-color: #35373c;
  background: #16171a;
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
  color: #b3b3ba;
}

.queue-title {
  font-size: 13px;
  font-weight: 500;
  color: #f4f4f5;
}

.queue-info {
  display: flex;
  justify-content: space-between;
  margin-top: 8px;
  font-size: 11px;
  color: #a1a1aa;
}

.load-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
  gap: 16px;
}

.load-card {
  background: #141416;
  border: 1px solid #232326;
  border-radius: 14px;
  padding: 12px;
  transition: border-color 0.18s ease, background 0.18s ease;
}

.load-card.saturated {
  border-color: rgba(239, 68, 68, 0.24);
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
  color: #f4f4f5;
}

.load-info {
  display: flex;
  justify-content: space-between;
  margin-top: 8px;
  font-size: 11px;
  color: #a1a1aa;
}

@media (max-width: 920px) {
  .queue-grid {
    grid-template-columns: 1fr;
  }
}
</style>
