<template>
  <div class="admin-page users-page">
    <div class="page-header">
      <div>
        <div class="title-row">
          <h2 class="page-title">用户控制台</h2>
        </div>
        <p class="page-desc">管理用户状态与账户生命周期。所有操作即时生效，并同步到任务权限链路。</p>
      </div>
      <div class="header-side-note">
        <n-space align="center" :size="20">
          <n-popover placement="bottom" trigger="click">
            <template #trigger>
              <n-button size="small" secondary>
                防刷限流设置
              </n-button>
            </template>
            <div style="width: 240px; padding: 4px;">
              <h3 style="margin: 0 0 12px; font-size: 14px;">登录/注册接口限流</h3>
              <n-form :model="rateLimitForm" size="small" label-placement="left" label-width="70">
                <n-form-item label="时间窗口">
                  <n-input-number v-model:value="rateLimitForm.window_seconds" :min="1" :max="3600">
                    <template #suffix>秒</template>
                  </n-input-number>
                </n-form-item>
                <n-form-item label="最大请求">
                  <n-input-number v-model:value="rateLimitForm.max_requests" :min="1" :max="10000">
                    <template #suffix>次</template>
                  </n-input-number>
                </n-form-item>
                <n-button type="primary" block :loading="rateLimitSaving" @click="handleSaveRateLimit">
                  保存设置
                </n-button>
              </n-form>
            </div>
          </n-popover>
          <n-space align="center" :size="6">
            <span>开放注册</span>
            <n-switch
              v-model:value="registrationEnabled"
              :loading="registrationSettingLoading"
              size="small"
              @update:value="handleToggleRegistration"
            />
          </n-space>
        </n-space>
      </div>
    </div>

    <div class="summary-grid">
      <AdminSummaryCard label="总用户" :value="users.length" foot="当前筛选结果" />
      <AdminSummaryCard label="正常" :value="activeUserCount" foot="状态可登录" />
      <AdminSummaryCard label="封禁" :value="bannedUserCount" foot="建议定期复核" />
      <AdminSummaryCard label="系统保留" :value="reservedUserCount" foot="不可操作账户" />
    </div>

    <div class="toolbar">
      <n-input
        v-model:value="searchKeyword"
        placeholder="搜索账号..."
        clearable
        style="width: 240px"
        @keyup.enter="loadUsers"
      >
        <template #prefix>
          <n-icon><SearchOutline /></n-icon>
        </template>
      </n-input>
      <n-button @click="loadUsers">
        <template #icon><n-icon><RefreshOutline /></n-icon></template>
        刷新
      </n-button>
      <n-button type="warning" ghost :loading="stopImpersonatingLoading" @click="handleStopImpersonating">
        停止伪装
      </n-button>
    </div>

    <div class="table-card">
      <AdminSectionHeader title="用户列表" description="支持封禁与解封、以及踢下线等身份侧管理动作。管理员账号不可删除。" />
      <n-data-table
        :columns="columns"
        :data="users"
        :loading="loading"
        :pagination="false"
        :bordered="false"
        :row-key="(row: User) => row.id"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, h, computed } from 'vue'
import { adminApi } from '../../../shared/api'
import { authClient, extractBetterAuthErrorMessage } from '../../../shared/auth/client'
import {
  NInput,
  NButton,
  NDataTable,
  NIcon,
  NTag,
  NSpace,
  NSwitch,
  NPopover,
  NInputNumber,
  NForm,
  NFormItem,
  useMessage,
  useDialog,
} from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import type { User } from '../../../shared/types'
import { SearchOutline, RefreshOutline } from '@vicons/ionicons5'
import { extractApiErrorMessage } from '../shared/assetLibraryShared'
import AdminSummaryCard from '../shared/components/AdminSummaryCard.vue'
import AdminSectionHeader from '../shared/components/AdminSectionHeader.vue'

const message = useMessage()
const dialog = useDialog()

const loading = ref(false)
const users = ref<User[]>([])
const searchKeyword = ref('')
const activeUserCount = computed(() => users.value.filter(item => item.status === 1).length)
const bannedUserCount = computed(() => users.value.filter(item => item.status !== 1).length)
const reservedUserCount = computed(() => users.value.filter(item => item.role === 'admin').length)

const stopImpersonatingLoading = ref(false)
const registrationEnabled = ref(false)
const registrationSettingLoading = ref(false)
const rateLimitForm = ref({ window_seconds: 60, max_requests: 30 })
const rateLimitSaving = ref(false)

const columns: DataTableColumns<User> = [
  { title: 'ID', key: 'id', width: 80 },
  { title: '账号', key: 'username' },
  {
    title: '状态',
    key: 'status',
    width: 100,
    render: (row) => h(NTag, { type: row.status === 1 ? 'success' : 'warning', size: 'small' }, () => row.status === 1 ? '正常' : '封禁')
  },
  { title: '创建时间', key: 'created_at', ellipsis: { tooltip: true } },
  {
    title: '操作',
    key: 'actions',
    width: 320,
    render: (row) => {
      const isSystemAdmin = row.role === 'admin'
      const authId = row.auth_user_id

      return h(NSpace, { size: 'small' }, () => [
        !isSystemAdmin
          ? h(NButton, { size: 'tiny', type: row.status === 1 ? 'warning' : 'success', onClick: () => handleToggleStatus(row) }, () => row.status === 1 ? '业务封禁' : '业务解封')
          : null,
        authId
          ? h(NButton, { size: 'tiny', type: 'error', ghost: true, onClick: () => handleRevokeSessions(row) }, () => '踢下线')
          : null,
        authId
          ? h(NButton, { size: 'tiny', type: 'primary', ghost: true, onClick: () => handleImpersonate(row) }, () => '伪装登录')
          : null,
      ].filter(Boolean))
    }
  }
]

async function loadUsers() {
  loading.value = true
  try {
    const res = await adminApi.getUsers({ keyword: searchKeyword.value, page: 1, page_size: 100 })
    if (res.code === 0) {
      users.value = res.data?.list || []
    }
  } catch {
    message.error('加载用户失败')
  } finally {
    loading.value = false
  }
}

async function handleToggleStatus(row: User) {
  const newStatus = row.status === 1 ? 0 : 1
  try {
    await adminApi.updateUserStatus(row.id, newStatus)
    message.success(newStatus === 1 ? '已解封' : '已封禁')
    loadUsers()
  } catch (error) {
    message.error(extractApiErrorMessage(error, newStatus === 1 ? '解封失败' : '封禁失败'))
  }
}

function handleRevokeSessions(row: User) {
  if (!row.auth_user_id) return
  const authId = row.auth_user_id

  dialog.warning({
    title: '确认踢下线',
    content: `将强制吊销账号 ${row.username} 的所有活跃会话，是否继续？`,
    positiveText: '确认',
    negativeText: '取消',
    onPositiveClick: async () => {
      try {
        const result = await authClient.admin.revokeUserSessions({ userId: authId })
        if ((result as any)?.error) throw (result as any).error
        message.success('已强制踢下线')
      } catch (error) {
        if (error && String(error).includes('revokeUserSessions is not a function')) {
          try {
            const resultFallback = await (authClient.admin as any).revokeUserSession({ userId: authId })
            if (resultFallback?.error) throw resultFallback.error
            message.success('已强制踢下线')
            return
          } catch (fallbackError) {
            message.error(extractBetterAuthErrorMessage(fallbackError, '踢下线失败'))
            return
          }
        }
        message.error(extractBetterAuthErrorMessage(error, '踢下线失败'))
      }
    },
  })
}

function handleImpersonate(row: User) {
  if (!row.auth_user_id) return
  const authId = row.auth_user_id

  dialog.warning({
    title: '确认伪装登录',
    content: `将切换到账号 ${row.username} 的身份会话，是否继续？`,
    positiveText: '确认',
    negativeText: '取消',
    onPositiveClick: async () => {
      try {
        const result = await authClient.admin.impersonateUser({ userId: authId })
        if ((result as any)?.error) throw (result as any).error
        message.success('会话已切换，正在跳转用户中心')
        window.location.href = '/user/center'
      } catch (error) {
        message.error(extractBetterAuthErrorMessage(error, '伪装登录失败'))
      }
    },
  })
}

async function handleStopImpersonating() {
  stopImpersonatingLoading.value = true
  try {
    const result = await authClient.admin.stopImpersonating()
    if ((result as any)?.error) throw (result as any).error
    message.success('已停止伪装会话')
    loadUsers()
  } catch (error) {
    message.error(extractBetterAuthErrorMessage(error, '停止伪装失败（或未处于伪装状态）'))
  } finally {
    stopImpersonatingLoading.value = false
  }
}

async function loadRegistrationSetting() {
  registrationSettingLoading.value = true
  try {
    const res = await adminApi.getRegistrationSetting()
    if (res.code === 0) {
      registrationEnabled.value = res.data?.enabled ?? false
    }
  } catch {
    message.error('加载注册状态失败')
  } finally {
    registrationSettingLoading.value = false
  }
}

async function handleToggleRegistration(newValue: boolean) {
  registrationSettingLoading.value = true
  try {
    const res = await adminApi.updateRegistrationSetting(newValue)
    if (res.code === 0) {
      registrationEnabled.value = res.data?.enabled ?? newValue
      message.success(`注册入口已${newValue ? '开放' : '关闭'}`)
    } else {
      throw new Error(res.msg)
    }
  } catch (error: any) {
    registrationEnabled.value = !newValue // 恢复原状
    message.error(error.message || '更新注册状态失败')
  } finally {
    registrationSettingLoading.value = false
  }
}

async function loadRateLimitSettings() {
  try {
    const res = await adminApi.getRateLimitSettings()
    if (res.code === 0 && res.data) {
      rateLimitForm.value.window_seconds = res.data.window_seconds
      rateLimitForm.value.max_requests = res.data.max_requests
    }
  } catch {
    message.error('加载频率限制配置失败')
  }
}

async function handleSaveRateLimit() {
  rateLimitSaving.value = true
  try {
    const res = await adminApi.updateRateLimitSettings(rateLimitForm.value)
    if (res.code === 0) {
      message.success('防刷限流设置已保存')
    } else {
      throw new Error(res.msg)
    }
  } catch (error: any) {
    message.error(error.message || '保存设置失败')
  } finally {
    rateLimitSaving.value = false
  }
}

onMounted(() => {
  loadUsers()
  loadRegistrationSetting()
  loadRateLimitSettings()
})
</script>

<style scoped>
@import '../shared/adminCommon.css';
.users-page {
  gap: 14px;
}

.title-row {
  display: flex;
  align-items: center;
  gap: 10px;
}

@media (max-width: 720px) {
  .users-page :deep(.n-input) {
    width: 100% !important;
  }

  .title-row {
    flex-wrap: wrap;
  }
}
</style>
