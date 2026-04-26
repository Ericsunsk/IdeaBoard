<template>
  <div class="admin-page users-page">
    <div class="page-header">
      <div>
        <h2 class="page-title">用户控制台</h2>
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
      <n-button type="primary" ghost :disabled="!authStudioURL" @click="openAuthStudio">
        身份安全管理
      </n-button>
    </div>

    <div class="table-card">
      <AdminSectionHeader title="用户列表" description="支持封禁与解封。管理员账号默认不可操作。" />
      <p class="ops-tip">账号安全操作（改密、会话吊销、物理删号）请在 Better Auth Studio 执行；此处仅做业务侧封禁治理。</p>
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
import { NInput, NButton, NDataTable, NIcon, NTag, NSpace, useMessage } from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import type { User } from '../../../shared/types'
import { SearchOutline, RefreshOutline } from '@vicons/ionicons5'
import { extractApiErrorMessage } from '../shared/assetLibraryShared'
import AdminSummaryCard from '../shared/components/AdminSummaryCard.vue'
import AdminSectionHeader from '../shared/components/AdminSectionHeader.vue'

const message = useMessage()

const loading = ref(false)
const users = ref<User[]>([])
const searchKeyword = ref('')
const activeUserCount = computed(() => users.value.filter(item => item.status === 1).length)
const bannedUserCount = computed(() => users.value.filter(item => item.status !== 1).length)
const reservedUserCount = computed(() => users.value.filter(item => item.role === 'admin').length)
const authStudioURL = (import.meta.env.VITE_AUTH_STUDIO_URL || '').trim()

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

function openAuthStudio() {
  if (!authStudioURL) {
    message.warning('未配置 VITE_AUTH_STUDIO_URL')
    return
  }
  window.open(authStudioURL, '_blank', 'noopener,noreferrer')
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

.ops-tip {
  margin: 10px 2px 12px;
  color: #5f6c86;
  font-size: 13px;
  line-height: 1.5;
}

@media (max-width: 720px) {
  .users-page :deep(.n-input) {
    width: 100% !important;
  }
}
</style>
