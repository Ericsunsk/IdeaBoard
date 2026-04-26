<template>
  <div class="admin-page users-page">
    <div class="page-header">
      <div>
        <div class="title-row">
          <h2 class="page-title">用户控制台</h2>
          <n-button type="primary" ghost size="small" @click="openAdvancedOptions">高级选项</n-button>
        </div>
        <p class="page-desc">管理用户状态与账户生命周期。所有操作即时生效，并同步到任务权限链路。</p>
      </div>
      <div class="header-side-note">
        <span>实时数据</span>
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
    </div>

    <div class="table-card">
      <AdminSectionHeader title="用户列表" description="支持封禁与解封。管理员账号默认不可操作。" />
      <n-data-table
        :columns="columns"
        :data="users"
        :loading="loading"
        :pagination="false"
        :bordered="false"
        :row-key="(row: User) => row.id"
      />
    </div>

    <n-modal
      v-model:show="advancedModalVisible"
      preset="card"
      title="高级选项"
      :style="{ width: 'min(1120px, calc(100vw - 40px))' }"
    >
      <div class="advanced-toolbar">
        <n-input
          v-model:value="authSearchValue"
          placeholder="搜索身份用户..."
          clearable
          style="width: 240px"
          @keyup.enter="loadAuthUsers"
        />
        <n-select
          v-model:value="authSearchField"
          style="width: 140px"
          :options="authSearchFieldOptions"
        />
        <n-button :loading="authUsersLoading" @click="loadAuthUsers">
          <template #icon><n-icon><RefreshOutline /></n-icon></template>
          刷新
        </n-button>
        <n-button type="warning" ghost :loading="stopImpersonatingLoading" @click="handleStopImpersonating">
          停止伪装
        </n-button>
      </div>
      <n-data-table
        :columns="authUserColumns"
        :data="authUsers"
        :loading="authUsersLoading"
        :pagination="false"
        :bordered="false"
        size="small"
        :max-height="460"
        :row-key="(row: BetterAuthAdminUser) => row.id"
      />
    </n-modal>
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
  NModal,
  NSelect,
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

type BetterAuthSearchField = 'email' | 'name'
type BetterAuthAdminUser = {
  id: string
  email: string
  name: string
  role: string
  banned: boolean
  banReason: string
  createdAt: string
}

const loading = ref(false)
const users = ref<User[]>([])
const searchKeyword = ref('')
const activeUserCount = computed(() => users.value.filter(item => item.status === 1).length)
const bannedUserCount = computed(() => users.value.filter(item => item.status !== 1).length)
const reservedUserCount = computed(() => users.value.filter(item => item.role === 'admin').length)
const advancedModalVisible = ref(false)
const authUsers = ref<BetterAuthAdminUser[]>([])
const authUsersLoading = ref(false)
const authSearchValue = ref('')
const authSearchField = ref<BetterAuthSearchField>('email')
const stopImpersonatingLoading = ref(false)
const authSearchFieldOptions = [
  { label: '按邮箱', value: 'email' },
  { label: '按名称', value: 'name' },
]

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
    width: 120,
    render: (row) => h(NSpace, { size: 'small' }, () => [
      row.role !== 'admin' ? h(NButton, { size: 'small', type: row.status === 1 ? 'warning' : 'success', onClick: () => handleToggleStatus(row) }, () => row.status === 1 ? '封禁' : '解封') : null
    ].filter(Boolean))
  }
]

const authUserColumns: DataTableColumns<BetterAuthAdminUser> = [
  { title: 'Email', key: 'email', ellipsis: { tooltip: true } },
  { title: 'Name', key: 'name', ellipsis: { tooltip: true } },
  { title: 'Role', key: 'role', width: 100 },
  {
    title: '状态',
    key: 'banned',
    width: 100,
    render: (row) =>
      h(NTag, { type: row.banned ? 'warning' : 'success', size: 'small' }, () => (row.banned ? '封禁' : '正常')),
  },
  { title: '封禁原因', key: 'banReason', ellipsis: { tooltip: true } },
  { title: '创建时间', key: 'createdAt', width: 180, ellipsis: { tooltip: true } },
  {
    title: '操作',
    key: 'actions',
    width: 210,
    render: (row) =>
      h(NSpace, { size: 'small' }, () => [
        row.banned
          ? h(NButton, { size: 'tiny', type: 'success', onClick: () => handleUnbanAuthUser(row) }, () => '解封')
          : h(NButton, { size: 'tiny', type: 'warning', onClick: () => handleBanAuthUser(row) }, () => '封禁'),
        h(NButton, { size: 'tiny', type: 'primary', ghost: true, onClick: () => handleImpersonate(row) }, () => '伪装登录'),
      ]),
  },
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

function openAdvancedOptions() {
  advancedModalVisible.value = true
  loadAuthUsers()
}

async function loadAuthUsers() {
  authUsersLoading.value = true
  try {
    const searchValue = authSearchValue.value.trim()
    const result = await authClient.admin.listUsers({
      query: {
        searchValue: searchValue || undefined,
        searchField: authSearchField.value,
        searchOperator: 'contains',
        limit: 50,
        offset: 0,
        sortBy: 'createdAt',
        sortDirection: 'desc',
      },
    })
    if ((result as any)?.error) {
      throw (result as any).error
    }
    authUsers.value = normalizeAuthUsersResult(result)
  } catch (error) {
    message.error(extractBetterAuthErrorMessage(error, '加载身份用户失败'))
  } finally {
    authUsersLoading.value = false
  }
}

function normalizeAuthUsersResult(result: unknown): BetterAuthAdminUser[] {
  const payload = result as any
  const list = payload?.data?.users ?? payload?.users ?? []
  if (!Array.isArray(list)) {
    return []
  }
  return list
    .map((item: any): BetterAuthAdminUser | null => {
      const id = String(item?.id || '').trim()
      if (!id) return null
      return {
        id,
        email: String(item?.email || ''),
        name: String(item?.name || ''),
        role: String(item?.role || 'user'),
        banned: Boolean(item?.banned),
        banReason: String(item?.banReason || ''),
        createdAt: formatDateTime(item?.createdAt),
      }
    })
    .filter((item: BetterAuthAdminUser | null): item is BetterAuthAdminUser => !!item)
}

async function handleBanAuthUser(row: BetterAuthAdminUser) {
  try {
    const result = await authClient.admin.banUser({
      userId: row.id,
      banReason: '管理员在高级选项中执行封禁',
    })
    if ((result as any)?.error) {
      throw (result as any).error
    }
    message.success('身份侧已封禁')
    loadAuthUsers()
  } catch (error) {
    message.error(extractBetterAuthErrorMessage(error, '身份侧封禁失败'))
  }
}

async function handleUnbanAuthUser(row: BetterAuthAdminUser) {
  try {
    const result = await authClient.admin.unbanUser({ userId: row.id })
    if ((result as any)?.error) {
      throw (result as any).error
    }
    message.success('身份侧已解封')
    loadAuthUsers()
  } catch (error) {
    message.error(extractBetterAuthErrorMessage(error, '身份侧解封失败'))
  }
}

function handleImpersonate(row: BetterAuthAdminUser) {
  dialog.warning({
    title: '确认伪装登录',
    content: `将切换到 ${row.email || row.name || row.id} 的身份会话，是否继续？`,
    positiveText: '确认',
    negativeText: '取消',
    onPositiveClick: async () => {
      try {
        const result = await authClient.admin.impersonateUser({ userId: row.id })
        if ((result as any)?.error) {
          throw (result as any).error
        }
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
    if ((result as any)?.error) {
      throw (result as any).error
    }
    message.success('已停止伪装会话')
    window.location.href = '/admin/users'
  } catch (error) {
    message.error(extractBetterAuthErrorMessage(error, '停止伪装失败'))
  } finally {
    stopImpersonatingLoading.value = false
  }
}

function formatDateTime(value: unknown): string {
  const text = String(value || '').trim()
  if (!text) return '-'
  const time = new Date(text)
  if (Number.isNaN(time.getTime())) return text
  return time.toLocaleString()
}

onMounted(() => {
  loadUsers()
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

.advanced-toolbar {
  margin-bottom: 12px;
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
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
