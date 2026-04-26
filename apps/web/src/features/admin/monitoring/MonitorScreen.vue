<template>
  <div class="admin-page monitor-page">
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
            :color="isAccountSaturated(load) ? ERROR_TEXT_COLOR : 'var(--admin-text-1)'"
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

    <div class="tasks-section tasks-shell">
      <div class="table-card admin-table-card">
        <div class="admin-card-header">
          <div class="admin-card-title-wrapper">
            <n-icon size="18" class="admin-card-title-icon"><ListOutline /></n-icon>
            <h3 class="admin-card-title">最近任务</h3>
          </div>
          <div class="admin-card-actions">
            <n-select v-model:value="filterStatus" :options="taskStatusOptions" placeholder="筛选状态" clearable style="width: 120px" />
            <n-button secondary class="icon-btn" @click="loadTasks">
              <template #icon><n-icon><RefreshOutline /></n-icon></template>
            </n-button>
          </div>
        </div>
        <n-data-table
          :columns="columns"
          :data="tasks"
          :loading="loading"
          :pagination="false"
          :bordered="false"
          size="small"
          class="admin-table-dense"
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
import accountApi from '../../../shared/api/account'
import type { Task } from '../../../shared/api/account'
import { NIcon, NTag, NProgress, NButton, NSelect, NDataTable, NPagination } from 'naive-ui'
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
import AdminSummaryCard from '../shared/components/AdminSummaryCard.vue'
import AdminSectionHeader from '../shared/components/AdminSectionHeader.vue'

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
let visibilityListenerAttached = false
const MAX_MONITOR_BACKOFF_MS = 60000
const MONITOR_HARD_PAUSE_MS = 120000
const MONITOR_FAILS_BEFORE_PAUSE = 6
const HIDDEN_PAGE_MIN_REFRESH_MS = 15000
const monitorConsecutiveFailures = ref(0)
const monitorPausedUntil = ref(0)
const monitorCurrentDelay = ref(MONITOR_REFRESH_INTERVAL_MS)

function clearRefreshTimer() {
  if (refreshTimer !== null) {
    clearTimeout(refreshTimer)
    refreshTimer = null
  }
}

function scheduleMonitorPoll(delayMs: number) {
  clearRefreshTimer()
  refreshTimer = window.setTimeout(() => {
    void pollMonitor()
  }, Math.max(0, delayMs))
}

async function loadMonitor(): Promise<boolean> {
  try {
    const res = await accountApi.getMonitor()
    if (res.code === 0) {
      monitor.value = res.data?.monitor || monitor.value
      stats.value = res.data?.stats || stats.value
      return true
    }
    return false
  } catch (e) {
    console.error('加载监控数据失败', e)
    return false
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
  void loadTasks()
})

async function pollMonitor(forceImmediate = false) {
  const now = Date.now()
  if (!forceImmediate && monitorPausedUntil.value > now) {
    scheduleMonitorPoll(monitorPausedUntil.value - now)
    return
  }

  const success = await loadMonitor()
  if (success) {
    monitorConsecutiveFailures.value = 0
    monitorPausedUntil.value = 0
    monitorCurrentDelay.value = MONITOR_REFRESH_INTERVAL_MS
  } else {
    monitorConsecutiveFailures.value += 1
    if (monitorConsecutiveFailures.value >= MONITOR_FAILS_BEFORE_PAUSE) {
      monitorPausedUntil.value = Date.now() + MONITOR_HARD_PAUSE_MS
      monitorCurrentDelay.value = MONITOR_HARD_PAUSE_MS
      monitorConsecutiveFailures.value = 0
    } else {
      monitorCurrentDelay.value = Math.min(
        MONITOR_REFRESH_INTERVAL_MS * (2 ** monitorConsecutiveFailures.value),
        MAX_MONITOR_BACKOFF_MS
      )
    }
  }

  const nextDelay = document.hidden
    ? Math.max(monitorCurrentDelay.value, HIDDEN_PAGE_MIN_REFRESH_MS)
    : monitorCurrentDelay.value
  scheduleMonitorPoll(nextDelay)
}

function onVisibilityChange() {
  if (!document.hidden) {
    void pollMonitor(true)
  }
}

onMounted(() => {
  void pollMonitor(true)
  void loadTasks()
  if (!visibilityListenerAttached) {
    document.addEventListener('visibilitychange', onVisibilityChange)
    visibilityListenerAttached = true
  }
})

onUnmounted(() => {
  clearRefreshTimer()
  if (visibilityListenerAttached) {
    document.removeEventListener('visibilitychange', onVisibilityChange)
    visibilityListenerAttached = false
  }
})
</script>

<style scoped>
@import '../shared/adminCommon.css';

.monitor-page {
  gap: 14px;
}

.tasks-shell {
  border: none;
  background: transparent;
  padding: 0;
}

.queue-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 16px;
}

.queue-card {
  background: var(--admin-card-strong);
  border: 1px solid var(--admin-border);
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
  color: var(--admin-text-2);
}

.queue-title {
  font-size: 13px;
  font-weight: 500;
  color: var(--admin-text-1);
}

.queue-info {
  display: flex;
  justify-content: space-between;
  margin-top: 8px;
  font-size: 11px;
  color: var(--admin-text-2);
}

.load-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
  gap: 16px;
}

.load-card {
  background: var(--admin-card-strong);
  border: 1px solid var(--admin-border);
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
  color: var(--admin-text-1);
}

.load-info {
  display: flex;
  justify-content: space-between;
  margin-top: 8px;
  font-size: 11px;
  color: var(--admin-text-2);
}

@media (max-width: 920px) {
  .queue-grid {
    grid-template-columns: 1fr;
  }
}
</style>
