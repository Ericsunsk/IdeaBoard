<template>
  <div class="users-page">
    <!-- 搜索栏 -->
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

    <!-- 用户列表 -->
    <div class="table-card">
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
import { ref, onMounted, h } from 'vue'
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
  padding: 28px 32px;
  display: flex;
  flex-direction: column;
  gap: 20px;
  max-width: 1400px;
}
.toolbar {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}
.table-card {
  background: #18181b;
  border: 1px solid #27272a;
  border-radius: 12px;
  padding: 20px;
}
:deep(.n-data-table) {
  --n-td-color: transparent;
  --n-th-color: transparent;
  --n-border-color: #27272a;
  --n-td-color-hover: rgba(99,102,241,0.05);
  --n-th-text-color: #71717a;
  --n-td-text-color: #e4e4e7;
}
:deep(.n-input) {
  --n-border: 1px solid #27272a;
  --n-border-hover: 1px solid #6366f1;
  --n-border-focus: 1px solid #6366f1;
  --n-color: #09090b;
  --n-color-focus: #09090b;
  --n-text-color: #e4e4e7;
  --n-placeholder-color: #3f3f46;
  border-radius: 8px;
}
:deep(.n-button--primary-type) {
  --n-color: #6366f1;
  --n-color-hover: #818cf8;
  --n-color-pressed: #4f46e5;
  border-radius: 8px;
}
</style>