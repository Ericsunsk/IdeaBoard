<template>
  <div class="center-page">
    <div class="header">
      <div class="header-content">
        <h1>个人中心</h1>
        <div class="header-actions">
          <span class="username">{{ userStore.user?.username }}</span>
          <n-button text @click="handleLogout">退出登录</n-button>
        </div>
      </div>
    </div>

    <div class="content">
      <div class="card">
        <h2>个人信息</h2>
        <div class="info-list">
          <div class="info-item">
            <span class="label">用户ID</span>
            <span class="value">{{ userStore.user?.id || '-' }}</span>
          </div>
          <div class="info-item">
            <span class="label">账号</span>
            <span class="value highlight">{{ userStore.user?.username || '-' }}</span>
          </div>
          <div class="info-item">
            <span class="label">角色</span>
            <n-tag :type="userStore.user?.role === 'admin' ? 'error' : 'success'" size="small">
              {{ userStore.user?.role === 'admin' ? '管理员' : '普通用户' }}
            </n-tag>
          </div>
          <div class="info-item">
            <span class="label">状态</span>
            <n-tag :type="userStore.user?.status === 1 ? 'success' : 'warning'" size="small">
              {{ userStore.user?.status === 1 ? '正常' : '封禁' }}
            </n-tag>
          </div>
          <div class="info-item">
            <span class="label">创建时间</span>
            <span class="value">{{ userStore.user?.created_at || '-' }}</span>
          </div>
        </div>
      </div>

      <div class="card">
        <h2>提交任务</h2>
        <n-form :model="taskForm" :rules="taskRules" ref="taskFormRef" label-placement="top">
          <n-form-item label="任务描述 (Prompt)" path="prompt">
            <n-input
              v-model:value="taskForm.prompt"
              type="textarea"
              placeholder="请输入绘图任务描述，例如：一只在月球上行走的猫咪，超写实风格"
              :rows="4"
            />
          </n-form-item>
          <n-button type="primary" :loading="taskLoading" :disabled="taskLoading" @click="handleSubmitTask">
            提交任务
          </n-button>
        </n-form>
      </div>

      <div class="card">
        <div class="card-header">
          <h2>我的任务</h2>
          <n-button size="small" @click="loadTasks">
            <template #icon><n-icon><RefreshOutline /></n-icon></template>
            刷新
          </n-button>
        </div>
        <n-data-table
          :columns="taskColumns"
          :data="tasks"
          :loading="taskLoading"
          :pagination="taskPagination"
          :bordered="false"
          size="small"
        />
      </div>

      <div class="card">
        <h2>修改密码</h2>
        <n-form ref="pwdFormRef" :model="pwdForm" :rules="pwdRules" size="medium">
          <n-form-item label="原密码" path="oldPassword">
            <n-input
              v-model:value="pwdForm.oldPassword"
              type="password"
              placeholder="请输入原密码"
              show-password-on="click"
            />
          </n-form-item>
          <n-form-item label="新密码" path="newPassword">
            <n-input
              v-model:value="pwdForm.newPassword"
              type="password"
              placeholder="请输入新密码"
              show-password-on="click"
            />
          </n-form-item>
          <n-form-item label="确认密码" path="confirmPassword">
            <n-input
              v-model:value="pwdForm.confirmPassword"
              type="password"
              placeholder="请再次输入新密码"
              show-password-on="click"
            />
          </n-form-item>
          <n-button type="primary" :loading="pwdLoading" @click="handleUpdatePassword">
            确认修改
          </n-button>
        </n-form>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, h } from 'vue'
import { useUserStore } from '../../stores/user'
import accountApi from '../../api/account'
import { NButton, NTag, NForm, NFormItem, NInput, NDataTable, NIcon, useMessage } from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import { RefreshOutline } from '@vicons/ionicons5'

interface TaskItem {
  id: number
  task_id: string
  status: string
  prompt: string
  retry_count?: number
  error_msg?: string
  created_at: string
  completed_at?: string | null
}

const userStore = useUserStore()
const message = useMessage()

const pwdFormRef = ref()
const pwdLoading = ref(false)
const pwdForm = ref({
  oldPassword: '',
  newPassword: '',
  confirmPassword: ''
})

const validateConfirmPwd = (_rule: any, value: string) => {
  if (value !== pwdForm.value.newPassword) {
    return new Error('两次输入的密码不一致')
  }
  return true
}

const pwdRules = {
  oldPassword: { required: true, message: '请输入原密码', trigger: 'blur' },
  newPassword: [
    { required: true, message: '请输入新密码', trigger: 'blur' },
    { min: 6, message: '密码至少6个字符', trigger: 'blur' }
  ],
  confirmPassword: [
    { required: true, message: '请再次输入新密码', trigger: 'blur' },
    { validator: validateConfirmPwd, trigger: 'blur' }
  ]
}

const taskFormRef = ref()
const taskLoading = ref(false)
const taskForm = ref({ prompt: '' })
const taskRules = {
  prompt: { required: true, message: '请输入任务描述', trigger: 'blur' }
}

const tasks = ref<TaskItem[]>([])
const taskPagination = ref({
  page: 1,
  pageSize: 10,
  total: 0,
  onChange: (page: number) => {
    (taskPagination.value as any).page = page
    loadTasks()
  }
})

const getStatusConfig = (status: string) => {
  switch (status) {
    case 'pending': return { type: 'default' as const, text: '等待中', color: '#71717a' }
    case 'running': return { type: 'warning' as const, text: '执行中', color: '#f59e0b' }
    case 'completed': return { type: 'success' as const, text: '已完成', color: '#22c55e' }
    case 'failed': return { type: 'error' as const, text: '失败', color: '#ef4444' }
    default: return { type: 'default' as const, text: status, color: '#71717a' }
  }
}

const taskColumns: DataTableColumns<TaskItem> = [
  { title: '任务ID', key: 'task_id', width: 200,
    render: (row) => h('span', { style: { fontFamily: 'monospace', fontSize: '11px', color: '#a1a1aa' } }, row.task_id) },
  { title: '状态', key: 'status', width: 90,
    render: (row) => {
      const cfg = getStatusConfig(row.status)
      return h(NTag, { type: cfg.type, size: 'small' }, () => cfg.text)
    } },
  { title: '重试次数', key: 'retry_count', width: 80, align: 'center' },
  { title: '描述', key: 'prompt', ellipsis: { tooltip: true } },
  { title: '创建时间', key: 'created_at', width: 160 },
]

async function loadTasks() {
  taskLoading.value = true
  try {
    const res = await accountApi.listTasks({
      page: (taskPagination.value as any).page,
      page_size: (taskPagination.value as any).pageSize,
    })
    if (res.code === 0) {
      tasks.value = res.data?.list || []
      ;(taskPagination.value as any).total = res.data?.total || 0
    }
  } catch {
    message.error('加载任务列表失败')
  } finally {
    taskLoading.value = false
  }
}

async function handleSubmitTask() {
  try {
    await taskFormRef.value?.validate()
    taskLoading.value = true
    const res = await accountApi.submitTask({ prompt: taskForm.value.prompt })
    if (res.code === 0) {
      message.success('任务提交成功！任务ID: ' + res.data?.task_id)
      taskForm.value.prompt = ''
      loadTasks()
    } else {
      message.error(res.msg || '提交失败')
    }
  } catch (error: any) {
    if (error.response?.data?.msg) {
      message.error(error.response.data.msg)
    } else if (error.message) {
      message.error(error.message)
    }
  } finally {
    taskLoading.value = false
  }
}

async function handleUpdatePassword() {
  try {
    await pwdFormRef.value?.validate()
    pwdLoading.value = true
    await userStore.updatePassword(pwdForm.value.oldPassword, pwdForm.value.newPassword)
    message.success('密码修改成功')
    pwdForm.value = { oldPassword: '', newPassword: '', confirmPassword: '' }
  } catch (error: any) {
    if (error.message) {
      message.error(error.message)
    }
  } finally {
    pwdLoading.value = false
  }
}

async function handleLogout() {
  await userStore.logout('/user/login')
}

onMounted(() => {
  userStore.fetchUserInfo()
  loadTasks()
})
</script>

<style scoped>
.center-page {
  min-height: 100vh;
  background: #09090b;
}

.header {
  background: #18181b;
  border-bottom: 1px solid #27272a;
  padding: 16px 24px;
}

.header-content {
  max-width: 900px;
  margin: 0 auto;
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.header-content h1 {
  font-size: 18px;
  font-weight: 600;
  color: #fafafa;
}

.header-actions {
  display: flex;
  align-items: center;
  gap: 16px;
}

.username {
  color: #a1a1aa;
  font-size: 14px;
}

.content {
  max-width: 900px;
  margin: 0 auto;
  padding: 32px 24px;
  display: flex;
  flex-direction: column;
  gap: 24px;
}

.card {
  background: #18181b;
  border: 1px solid #27272a;
  border-radius: 12px;
  padding: 28px;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 20px;
}

.card h2 {
  font-size: 16px;
  font-weight: 600;
  color: #fafafa;
  margin-bottom: 20px;
}

.card-header h2 {
  margin-bottom: 0;
}

.info-list {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.info-item {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding-bottom: 16px;
  border-bottom: 1px solid #27272a;
}

.info-item:last-child {
  border-bottom: none;
  padding-bottom: 0;
}

.label {
  color: #71717a;
  font-size: 14px;
}

.value {
  color: #fafafa;
  font-size: 14px;
}

.value.highlight {
  color: #6366f1;
  font-weight: 500;
}

:deep(.n-form-item) {
  margin-bottom: 16px;
}

:deep(.n-form-item-label__text) {
  font-size: 14px;
  color: #a1a1aa;
}

:deep(.n-input) {
  --n-border: 1px solid #3f3f46;
  --n-border-hover: 1px solid #6366f1;
  --n-border-focus: 1px solid #6366f1;
  --n-color: #27272a;
  --n-color-focus: #27272a;
  --n-text-color: #fafafa;
  --n-placeholder-color: #71717a;
  --n-caret-color: #6366f1;
  border-radius: 8px;
}

:deep(.n-button--primary-type) {
  --n-color: #6366f1;
  --n-color-hover: #818cf8;
  --n-color-pressed: #4f46e5;
  --n-color-focus: #6366f1;
  height: 40px;
  border-radius: 8px;
  margin-top: 8px;
}
</style>
