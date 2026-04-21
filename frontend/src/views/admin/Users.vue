<template>
  <div class="admin-page users-page">
    <div class="page-header">
      <div>
        <h2 class="page-title">用户控制台</h2>
        <p class="page-desc">管理用户状态、密码与账户生命周期。所有操作即时生效，并同步到任务权限链路。</p>
      </div>
      <div class="header-side-note">
        <span>实时数据</span>
      </div>
    </div>

    <div class="summary-grid">
      <div class="summary-card">
        <span class="summary-label">总用户</span>
        <span class="summary-value">{{ users.length }}</span>
        <span class="summary-foot">当前筛选结果</span>
      </div>
      <div class="summary-card">
        <span class="summary-label">正常</span>
        <span class="summary-value">{{ activeUserCount }}</span>
        <span class="summary-foot">状态可登录</span>
      </div>
      <div class="summary-card">
        <span class="summary-label">封禁</span>
        <span class="summary-value">{{ bannedUserCount }}</span>
        <span class="summary-foot">建议定期复核</span>
      </div>
      <div class="summary-card">
        <span class="summary-label">系统保留</span>
        <span class="summary-value">{{ reservedUserCount }}</span>
        <span class="summary-foot">不可删除账户</span>
      </div>
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
      <n-button type="primary" @click="showAddModal = true">
        <template #icon><n-icon><AddOutline /></n-icon></template>
        新增用户
      </n-button>
    </div>

    <div class="table-card">
      <div class="panel-header">
        <div>
          <h3 class="panel-title">用户列表</h3>
          <p class="panel-desc">支持封禁、改密、删除操作。管理员主账号默认不可删除。</p>
        </div>
      </div>
      <n-data-table
        :columns="columns"
        :data="users"
        :loading="loading"
        :pagination="false"
        :bordered="false"
        :row-key="(row: User) => row.id"
      />
    </div>

    <!-- 新增用户弹窗 -->
    <n-modal v-model:show="showAddModal" preset="card" title="新增用户" style="width: 400px">
      <n-form :model="addForm" :rules="addRules" ref="addFormRef" label-placement="top">
        <n-form-item label="账号" path="username">
          <n-input v-model:value="addForm.username" placeholder="请输入账号" />
        </n-form-item>
        <n-form-item label="密码" path="password">
          <n-input v-model:value="addForm.password" type="password" placeholder="请输入密码" show-password-on="click" />
        </n-form-item>
      </n-form>
      <template #footer>
        <n-space justify="end">
          <n-button @click="showAddModal = false">取消</n-button>
          <n-button type="primary" :loading="addLoading" @click="handleAddUser">确定</n-button>
        </n-space>
      </template>
    </n-modal>

    <!-- 修改密码弹窗 -->
    <n-modal v-model:show="showPwdModal" preset="card" title="修改密码" style="width: 400px">
      <n-form :model="pwdForm" :rules="pwdRules" ref="pwdFormRef" label-placement="top">
        <n-form-item label="新密码" path="new_password">
          <n-input v-model:value="pwdForm.new_password" type="password" placeholder="请输入新密码" show-password-on="click" />
        </n-form-item>
      </n-form>
      <template #footer>
        <n-space justify="end">
          <n-button @click="showPwdModal = false">取消</n-button>
          <n-button type="primary" :loading="pwdLoading" @click="handleUpdatePassword">确定</n-button>
        </n-space>
      </template>
    </n-modal>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, h, computed } from 'vue'
import { adminApi } from '../../api'
import { NInput, NButton, NDataTable, NModal, NForm, NFormItem, NIcon, NTag, NSpace, useMessage, useDialog } from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import type { User } from '../../types'
import { SearchOutline, RefreshOutline, AddOutline } from '@vicons/ionicons5'

const message = useMessage()
const dialog = useDialog()

const loading = ref(false)
const users = ref<User[]>([])
const searchKeyword = ref('')

const showAddModal = ref(false)
const addFormRef = ref()
const addLoading = ref(false)
const addForm = ref({ username: '', password: '' })
const addRules = {
  username: [{ required: true, message: '请输入账号', trigger: 'blur' }],
  password: [{ required: true, message: '请输入密码', trigger: 'blur' }]
}

const showPwdModal = ref(false)
const pwdFormRef = ref()
const pwdLoading = ref(false)
const pwdForm = ref({ new_password: '' })
const pwdRules = {
  new_password: [{ required: true, message: '请输入新密码', trigger: 'blur' }]
}

const currentUserId = ref<number>(0)
const activeUserCount = computed(() => users.value.filter(item => item.status === 1).length)
const bannedUserCount = computed(() => users.value.filter(item => item.status !== 1).length)
const reservedUserCount = computed(() => users.value.filter(item => item.id === 1).length)

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
    width: 180,
    render: (row) => h(NSpace, { size: 'small' }, () => [
      h(NButton, { size: 'small', type: row.status === 1 ? 'warning' : 'success', onClick: () => handleToggleStatus(row) }, () => row.status === 1 ? '封禁' : '解封'),
      h(NButton, { size: 'small', onClick: () => openPwdModal(row.id) }, () => '改密'),
      row.id !== 1 ? h(NButton, { size: 'small', type: 'error', onClick: () => handleDelete(row) }, () => '删除') : null
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

async function handleAddUser() {
  try {
    await addFormRef.value?.validate()
    addLoading.value = true
    await adminApi.createUser(addForm.value)
    message.success('添加成功')
    showAddModal.value = false
    addForm.value = { username: '', password: '' }
    loadUsers()
  } catch (error: any) {
    message.error(error.message || '添加失败')
  } finally {
    addLoading.value = false
  }
}

async function handleToggleStatus(row: User) {
  const newStatus = row.status === 1 ? 0 : 1
  await adminApi.updateUserStatus(row.id, newStatus)
  message.success(newStatus === 1 ? '已解封' : '已封禁')
  loadUsers()
}

function openPwdModal(id: number) {
  currentUserId.value = id
  pwdForm.value.new_password = ''
  showPwdModal.value = true
}

async function handleUpdatePassword() {
  try {
    await pwdFormRef.value?.validate()
    pwdLoading.value = true
    await adminApi.updateUserPassword(currentUserId.value, pwdForm.value.new_password)
    message.success('密码修改成功')
    showPwdModal.value = false
  } catch (error: any) {
    message.error(error.message || '修改失败')
  } finally {
    pwdLoading.value = false
  }
}

async function handleDelete(row: User) {
  dialog.warning({
    title: '确认删除',
    content: `确定删除用户 "${row.username}" 吗？`,
    positiveText: '确定',
    negativeText: '取消',
    onPositiveClick: async () => {
      await adminApi.deleteUser(row.id)
      message.success('删除成功')
      loadUsers()
    }
  })
}

onMounted(() => {
  loadUsers()
})
</script>

<style scoped>
@import './admin-common.css';
.users-page {
  gap: 14px;
}

@media (max-width: 720px) {
  .users-page :deep(.n-input) {
    width: 100% !important;
  }
}
</style>
