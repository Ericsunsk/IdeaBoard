<template>
  <div class="admin-page accounts-page">
    <div class="page-header">
      <div>
        <h2 class="page-title">账号池控制中心</h2>
        <p class="page-desc">管理上游账号并发、超时与启停状态，保障任务队列稳定吞吐。</p>
      </div>
      <div class="header-side-note">
        <span>分页 {{ page }}/{{ pageCount }}</span>
      </div>
    </div>

    <div class="summary-grid">
      <div class="summary-card">
        <span class="summary-label">账号总数</span>
        <span class="summary-value">{{ accounts.length }}</span>
        <span class="summary-foot">当前页数据</span>
      </div>
      <div class="summary-card">
        <span class="summary-label">运行中</span>
        <span class="summary-value">{{ runningCount }}</span>
        <span class="summary-foot">status = 1</span>
      </div>
      <div class="summary-card">
        <span class="summary-label">已停止</span>
        <span class="summary-value">{{ stoppedCount }}</span>
        <span class="summary-foot">手动停用</span>
      </div>
      <div class="summary-card">
        <span class="summary-label">故障账号</span>
        <span class="summary-value">{{ failedCount }}</span>
        <span class="summary-foot">失败次数 ≥ 5</span>
      </div>
    </div>

    <div class="toolbar">
      <n-input
        v-model:value="searchKeyword"
        placeholder="搜索账号名称..."
        clearable
        style="width: 200px"
        @keyup.enter="loadAccounts"
      >
        <template #prefix>
          <n-icon><SearchOutline /></n-icon>
        </template>
      </n-input>
      <n-button @click="loadAccounts">
        <template #icon><n-icon><RefreshOutline /></n-icon></template>
        刷新
      </n-button>
      <n-button type="primary" @click="openAddModal">
        <template #icon><n-icon><AddOutline /></n-icon></template>
        新增账号
      </n-button>
    </div>

    <div class="table-card">
      <div class="panel-header">
        <div>
          <h3 class="panel-title">账号列表</h3>
          <p class="panel-desc">可编辑上游密钥、模型、并发和超时配置。</p>
        </div>
      </div>
      <n-data-table
        :columns="columns"
        :data="accounts"
        :loading="loading"
        :pagination="false"
        :bordered="false"
        :row-key="(row: ApiAccount) => row.id"
      />
      <div class="pagination-wrapper">
        <n-pagination
          v-model:page="page"
          :page-count="pageCount"
          @update:page="loadAccounts"
        />
      </div>
    </div>

    <!-- 新增账号弹窗：先选渠道商 → 再填 Key/并发/超时 -->
    <n-modal v-model:show="showAddModal" preset="card" title="新增账号" style="width: 500px">
      <n-form :model="addFormData" :rules="addFormRules" ref="addFormRef" label-placement="top">
        <n-form-item label="选择渠道商" path="channel_id">
          <n-select
            v-model:value="addFormData.channel_id"
            :options="channelOptions"
            placeholder="请先选择渠道商"
            :loading="channelsLoading"
          />
        </n-form-item>

        <div v-if="selectedChannelUrl" class="channel-url-hint">
          <span>API 地址：</span>
          <code>{{ selectedChannelUrl }}</code>
        </div>

        <n-form-item label="使用模型" path="model_set">
          <n-select
            v-model:value="addFormData.model_set"
            :options="addModelOptions"
            placeholder="请先选择渠道商"
            :disabled="!addFormData.channel_id"
          />
        </n-form-item>

        <n-form-item label="API 密钥 (Key)" path="api_key">
          <n-input v-model:value="addFormData.api_key" type="password" show-password-on="click" placeholder="输入此渠道商的 API Key" />
        </n-form-item>

        <n-grid :cols="2" :x-gap="16">
          <n-gi>
            <n-form-item label="最大并发数" path="max_concurrent">
              <n-input-number v-model:value="addFormData.max_concurrent" :min="1" :max="20" style="width: 100%" />
            </n-form-item>
          </n-gi>
          <n-gi>
            <n-form-item label="超时时间（秒）" path="timeout">
              <n-input-number v-model:value="addFormData.timeout" :min="10" :max="600" style="width: 100%" />
            </n-form-item>
          </n-gi>
        </n-grid>
      </n-form>
      <template #footer>
        <n-space justify="end">
          <n-button @click="showAddModal = false">取消</n-button>
          <n-button type="primary" :loading="saveLoading" @click="handleAdd">确定</n-button>
        </n-space>
      </template>
    </n-modal>

    <!-- 编辑账号弹窗 -->
    <n-modal v-model:show="showEditModal" preset="card" title="编辑账号" style="width: 500px">
      <n-form :model="editFormData" ref="editFormRef" label-placement="top">
        <n-form-item label="所属渠道商">
          <n-select
            v-model:value="editFormData.channel_id"
            :options="channelOptions"
            placeholder="选择渠道商"
          />
        </n-form-item>

        <n-form-item label="使用模型" path="model_set">
          <n-select
            v-model:value="editFormData.model_set"
            :options="editModelOptions"
            placeholder="选择模型"
            :disabled="!editFormData.channel_id"
          />
        </n-form-item>

        <n-form-item label="修改 API 密钥（留空不修改）">
          <n-input v-model:value="editFormData.api_key" type="password" show-password-on="click" placeholder="留空则不修改" />
        </n-form-item>

        <n-grid :cols="2" :x-gap="16">
          <n-gi>
            <n-form-item label="最大并发数">
              <n-input-number v-model:value="editFormData.max_concurrent" :min="1" :max="20" style="width: 100%" />
            </n-form-item>
          </n-gi>
          <n-gi>
            <n-form-item label="超时时间（秒）">
              <n-input-number v-model:value="editFormData.timeout" :min="10" :max="600" style="width: 100%" />
            </n-form-item>
          </n-gi>
        </n-grid>

        <n-form-item label="状态">
          <n-radio-group v-model:value="editFormData.status">
            <n-radio :value="1">启用</n-radio>
            <n-radio :value="0">禁用</n-radio>
          </n-radio-group>
        </n-form-item>
      </n-form>
      <template #footer>
        <n-space justify="end">
          <n-button @click="showEditModal = false">取消</n-button>
          <n-button type="primary" :loading="saveLoading" @click="handleEdit">保存</n-button>
        </n-space>
      </template>
    </n-modal>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted, h } from 'vue'
import accountApi from '../../api/account'
import channelApi from '../../api/channel'
import type { Channel } from '../../api/channel'
import { NInput, NButton, NDataTable, NModal, NForm, NFormItem, NIcon, NTag, NSpace, NSelect, NRadioGroup, NRadio, NInputNumber, NPagination, NGrid, NGi, useMessage, useDialog } from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import type { ApiAccount } from '../../api/account'
import { SearchOutline, RefreshOutline, AddOutline } from '@vicons/ionicons5'

const message = useMessage()
const dialog = useDialog()

// === 渠道商 ===
const channels = ref<Channel[]>([])
const channelsLoading = ref(false)
const channelOptions = computed(() =>
  channels.value.map(ch => ({ label: ch.name, value: ch.id }))
)

async function loadChannels() {
  channelsLoading.value = true
  try {
    const res = await channelApi.list()
    if (res.code === 0) channels.value = res.data?.list || []
  } catch {} finally { channelsLoading.value = false }
}

// === 账号列表 ===
const loading = ref(false)
const accounts = ref<ApiAccount[]>([])
const searchKeyword = ref('')
const page = ref(1)
const pageCount = ref(1)
const runningCount = computed(() => accounts.value.filter(item => item.status === 1 && item.fail_count < 5).length)
const stoppedCount = computed(() => accounts.value.filter(item => item.status === 0).length)
const failedCount = computed(() => accounts.value.filter(item => item.fail_count >= 5).length)

const getChannelName = (channelId: number) => {
  const ch = channels.value.find(c => c.id === channelId)
  return ch?.name || (channelId ? `#${channelId}` : '-')
}

const getStatusType = (status: number, failCount: number) => {
  if (status === 0) return 'warning'
  if (failCount >= 5) return 'error'
  return 'success'
}

const getStatusText = (status: number, failCount: number) => {
  if (status === 0) return '已停止'
  if (failCount >= 5) return '故障'
  return '运行中'
}

const columns: DataTableColumns<ApiAccount> = [
  { title: 'ID', key: 'id', width: 60 },
  {
    title: '渠道商', key: 'channel_id', width: 140,
    render: (row) => h(NTag, { type: 'info', size: 'small', bordered: false },
      () => getChannelName((row as any).channel_id))
  },
  {
    title: '模型', key: 'model_set', width: 130,
    render: (row) => h(NTag, { size: 'small', type: 'success', bordered: false },
      () => (row as any).model_set || '-')
  },
  { title: '最大并发', key: 'max_concurrent', width: 90, align: 'center' },
  {
    title: '当前并发', key: 'current_concurrent', width: 90, align: 'center',
    render: (row) => h('span', { style: { color: (row.current_concurrent || 0) >= row.max_concurrent ? '#f59e0b' : '#22c55e' } },
      `${row.current_concurrent || 0}/${row.max_concurrent}`)
  },
  { title: '超时(s)', key: 'timeout', width: 80, align: 'center' },
  {
    title: '状态', key: 'status', width: 90,
    render: (row) => h(NTag, { type: getStatusType(row.status, row.fail_count), size: 'small' },
      () => getStatusText(row.status, row.fail_count))
  },
  { title: '失败次数', key: 'fail_count', width: 80, align: 'center' },
  { title: '创建时间', key: 'created_at', width: 170, ellipsis: { tooltip: true } },
  {
    title: '操作',
    key: 'actions',
    width: 200,
    render: (row) => h(NSpace, { size: 'small' }, () => [
      h(NButton, { size: 'small', onClick: () => openEditModal(row) }, () => '编辑'),
      row.status === 1
        ? h(NButton, { size: 'small', type: 'warning', onClick: () => handleStop(row) }, () => '停止')
        : h(NButton, { size: 'small', type: 'success', onClick: () => handleStart(row) }, () => '启动'),
      h(NButton, { size: 'small', type: 'error', onClick: () => handleDelete(row) }, () => '删除'),
    ])
  }
]

async function loadAccounts() {
  loading.value = true
  try {
    const res = await accountApi.listAccounts({
      keyword: searchKeyword.value || undefined,
      page: page.value,
      page_size: 20,
    })
    if (res.code === 0) {
      accounts.value = res.data?.list || []
      pageCount.value = Math.ceil((res.data?.total || 0) / 20)
    }
  } catch (e: any) {
    message.error(e.msg || '加载账号列表失败')
  } finally {
    loading.value = false
  }
}

// === 新增账号 ===
const showAddModal = ref(false)
const addFormRef = ref()
const saveLoading = ref(false)

const addFormData = ref({
  channel_id: null as number | null,
  model_set: '' as string,
  api_key: '',
  max_concurrent: 3,
  timeout: 120,
})

const selectedChannelUrl = computed(() => {
  if (!addFormData.value.channel_id) return ''
  const ch = channels.value.find(c => c.id === addFormData.value.channel_id)
  return ch?.api_url || ''
})

const addModelOptions = computed(() => {
  if (!addFormData.value.channel_id) return []
  const ch = channels.value.find(c => c.id === addFormData.value.channel_id)
  if (!ch?.models) return []
  return ch.models.split(',').filter(Boolean).map(m => ({ label: m, value: m }))
})

const editModelOptions = computed(() => {
  if (!editFormData.value.channel_id) return []
  const ch = channels.value.find(c => c.id === editFormData.value.channel_id)
  if (!ch?.models) return []
  return ch.models.split(',').filter(Boolean).map(m => ({ label: m, value: m }))
})

watch(() => addFormData.value.channel_id, () => {
  addFormData.value.model_set = ''
})

const addFormRules = {
  channel_id: [{ required: true, type: 'number' as const, message: '请选择渠道商', trigger: 'change' }],
  model_set: [{ required: true, message: '请选择使用模型', trigger: 'change' }],
  api_key: [{ required: true, message: '请输入 API 密钥', trigger: 'blur' }],
}

function openAddModal() {
  addFormData.value = { channel_id: null, model_set: '', api_key: '', max_concurrent: 3, timeout: 120 }
  showAddModal.value = true
}

async function handleAdd() {
  try {
    await addFormRef.value?.validate()
    saveLoading.value = true

    const ch = channels.value.find(c => c.id === addFormData.value.channel_id)
    const data = {
      channel_id: addFormData.value.channel_id!,
      model_set: addFormData.value.model_set,
      api_key: addFormData.value.api_key,
      max_concurrent: addFormData.value.max_concurrent,
      timeout: addFormData.value.timeout,
      name: ch?.name || '',
      platform: ch?.name || '',
      api_type: 'official',
      proxy_url: ch?.api_url || '',
    }
    await accountApi.createAccount(data as any)
    message.success('账号创建成功')
    showAddModal.value = false
    loadAccounts()
  } catch (e: any) {
    message.error(e.msg || e.message || '创建失败')
  } finally {
    saveLoading.value = false
  }
}

// === 编辑账号 ===
const showEditModal = ref(false)
const editFormRef = ref()
const currentEditId = ref(0)

const editFormData = ref({
  channel_id: null as number | null,
  model_set: '' as string,
  api_key: '',
  max_concurrent: 3,
  timeout: 120,
  status: 1 as number,
})

watch(() => editFormData.value.channel_id, () => {
  editFormData.value.model_set = ''
})

function openEditModal(row: ApiAccount) {
  currentEditId.value = row.id
  editFormData.value = {
    channel_id: (row as any).channel_id || null,
    model_set: (row as any).model_set || '',
    api_key: '',
    max_concurrent: row.max_concurrent,
    timeout: row.timeout,
    status: row.status,
  }
  showEditModal.value = true
}

async function handleEdit() {
  try {
    saveLoading.value = true
    const ch = channels.value.find(c => c.id === editFormData.value.channel_id)
    const data: any = {
      channel_id: editFormData.value.channel_id || undefined,
      model_set: editFormData.value.model_set,
      max_concurrent: editFormData.value.max_concurrent,
      timeout: editFormData.value.timeout,
      status: editFormData.value.status,
      name: ch?.name || '',
      platform: ch?.name || '',
      api_type: 'official',
      proxy_url: ch?.api_url || '',
    }
    if (editFormData.value.api_key) {
      data.api_key = editFormData.value.api_key
    }
    await accountApi.updateAccount(currentEditId.value, data)
    message.success('更新成功')
    showEditModal.value = false
    loadAccounts()
  } catch (e: any) {
    message.error(e.msg || e.message || '更新失败')
  } finally {
    saveLoading.value = false
  }
}

// === 启停/删除 ===
async function handleStart(row: ApiAccount) {
  await accountApi.startAccount(row.id)
  message.success('账号已启动')
  loadAccounts()
}

async function handleStop(row: ApiAccount) {
  await accountApi.stopAccount(row.id)
  message.success('账号已停止')
  loadAccounts()
}

async function handleDelete(row: ApiAccount) {
  dialog.warning({
    title: '确认删除',
    content: `确定删除此账号吗？`,
    positiveText: '确定',
    negativeText: '取消',
    onPositiveClick: async () => {
      await accountApi.deleteAccount(row.id)
      message.success('删除成功')
      loadAccounts()
    }
  })
}

onMounted(() => {
  loadChannels()
  loadAccounts()
})
</script>

<style scoped>
@import './admin-common.css';
.accounts-page {
  gap: 14px;
}
.pagination-wrapper {
  margin-top: 12px;
}
.channel-url-hint {
  background: rgba(113, 97, 239, 0.08);
  border-radius: 14px;
  border: 1px solid rgba(113, 97, 239, 0.2);
  padding: 9px 12px;
  margin-bottom: 12px;
  font-size: 12px;
  color: #64709a;
}
.channel-url-hint code {
  color: #4f58cc;
  font-family: monospace;
}

@media (max-width: 720px) {
  .accounts-page :deep(.n-input) {
    width: 100% !important;
  }
}
</style>
