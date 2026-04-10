<template>
  <div class="channels-page">
    <div class="toolbar">
      <h3>渠道商管理</h3>
      <n-button type="primary" @click="openAddModal">
        <template #icon><n-icon><AddOutline /></n-icon></template>
        新增渠道商
      </n-button>
    </div>

    <div class="table-card">
      <n-data-table
        :columns="columns"
        :data="channels"
        :loading="loading"
        :pagination="false"
        :bordered="false"
        :row-key="(row: Channel) => row.id"
        size="small"
      />
    </div>

    <n-modal v-model:show="showModal" preset="card" :title="isEdit ? '编辑渠道商' : '新增渠道商'" style="width: 480px">
      <n-form :model="formData" :rules="formRules" ref="formRef" label-placement="top">
        <n-form-item label="渠道类型" path="type">
          <n-select v-model:value="formData.type" :options="channelTypeOptions" placeholder="选择渠道类型" />
        </n-form-item>
        <n-form-item label="渠道商名称" path="name">
          <n-input v-model:value="formData.name" placeholder="例如：Nano Banana 官方" />
        </n-form-item>
        <n-form-item label="API 地址" path="api_url">
          <n-input v-model:value="formData.api_url" placeholder="例如：https://api.example.com/v1" />
        </n-form-item>
        <n-form-item label="可用模型" path="models">
          <n-dynamic-tags v-model:value="modelTags" />
          <div style="font-size: 12px; color: #71717a; margin-top: 4px">输入模型名后按回车添加，如 dall-e-3、gpt-image-1</div>
        </n-form-item>
      </n-form>
      <template #footer>
        <n-space justify="end">
          <n-button @click="showModal = false">取消</n-button>
          <n-button type="primary" :loading="saveLoading" @click="handleSave">确定</n-button>
        </n-space>
      </template>
    </n-modal>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, h } from 'vue'
import channelApi from '../../api/channel'
import type { Channel } from '../../api/channel'
import { NButton, NDataTable, NModal, NForm, NFormItem, NInput, NIcon, NSpace, NTag, NSelect, NDynamicTags, useMessage, useDialog } from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import { AddOutline } from '@vicons/ionicons5'

const message = useMessage()
const dialog = useDialog()
const loading = ref(false)
const channels = ref<Channel[]>([])

const showModal = ref(false)
const isEdit = ref(false)
const formRef = ref()
const saveLoading = ref(false)
const currentId = ref(0)

const channelTypeOptions = [
  { label: 'Banana', value: 'banana' },
]

const formData = ref({ type: 'banana', name: '', api_url: '', models: '' })
const modelTags = ref<string[]>([])

const formRules = {
  type: [{ required: true, message: '请选择渠道类型', trigger: 'change' }],
  name: [{ required: true, message: '请输入渠道商名称', trigger: 'blur' }],
  api_url: [{ required: true, message: '请输入 API 地址', trigger: 'blur' }],
  models: [{ required: true, message: '请添加至少一个可用模型', validator: () => modelTags.value.length > 0 ? true : new Error('请添加至少一个可用模型') }],
}

const getTypeLabel = (type: string) => {
  const opt = channelTypeOptions.find(o => o.value === type)
  return opt?.label || type
}

const columns: DataTableColumns<Channel> = [
  { title: 'ID', key: 'id', width: 70 },
  {
    title: '渠道类型', key: 'channel_type', width: 120,
    render: (row) => h(NTag, { type: 'warning', size: 'small', bordered: false }, () => getTypeLabel(row.type))
  },
  { title: '渠道商名称', key: 'name', width: 200 },
  {
    title: '可用模型', key: 'models', width: 200,
    render: (row) => {
      const models = row.models ? row.models.split(',').filter(Boolean) : []
      return models.length
        ? h(NSpace, { size: 'small', wrap: true }, () => models.map(m => h(NTag, { size: 'small', type: 'info', bordered: false }, () => m)))
        : h('span', { style: { color: '#71717a' } }, '未设置')
    }
  },
  {
    title: 'API 地址',
    key: 'api_url',
    ellipsis: { tooltip: true },
    render: (row) => h('span', { style: { fontFamily: 'monospace', fontSize: '12px' } }, row.api_url),
  },
  { title: '创建时间', key: 'created_at', width: 170 },
  {
    title: '操作',
    key: 'actions',
    width: 160,
    render: (row) => h(NSpace, { size: 'small' }, () => [
      h(NButton, { size: 'small', onClick: () => openEditModal(row) }, () => '编辑'),
      h(NButton, { size: 'small', type: 'error', onClick: () => handleDelete(row) }, () => '删除'),
    ]),
  },
]

async function loadChannels() {
  loading.value = true
  try {
    const res = await channelApi.list()
    if (res.code === 0) {
      channels.value = res.data?.list || []
    }
  } catch (e: any) {
    message.error(e.msg || '加载渠道商列表失败')
  } finally {
    loading.value = false
  }
}

function openAddModal() {
  isEdit.value = false
  formData.value = { type: 'banana', name: '', api_url: '', models: '' }
  modelTags.value = []
  showModal.value = true
}

function openEditModal(row: Channel) {
  isEdit.value = true
  currentId.value = row.id
  formData.value = { type: row.type, name: row.name, api_url: row.api_url, models: row.models || '' }
  modelTags.value = row.models ? row.models.split(',').filter(Boolean) : []
  showModal.value = true
}

async function handleSave() {
  try {
    await formRef.value?.validate()
    saveLoading.value = true

    const submitData = { ...formData.value, models: modelTags.value.join(',') }
    if (isEdit.value) {
      await channelApi.update(currentId.value, submitData)
      message.success('更新成功')
    } else {
      await channelApi.create(submitData)
      message.success('创建成功')
    }
    showModal.value = false
    await loadChannels()
  } catch (e: any) {
    message.error(e.msg || e.message || '操作失败')
  } finally {
    saveLoading.value = false
  }
}

async function handleDelete(row: Channel) {
  dialog.warning({
    title: '确认删除',
    content: `确定删除渠道商「${row.name}」吗？`,
    positiveText: '确定',
    negativeText: '取消',
    onPositiveClick: async () => {
      await channelApi.delete(row.id)
      message.success('删除成功')
      loadChannels()
    },
  })
}

onMounted(() => {
  loadChannels()
})
</script>

<style scoped>
@import './admin-common.css';
.channels-page {
  padding: 28px 32px;
  display: flex;
  flex-direction: column;
  gap: 20px;
  max-width: 1400px;
}
.toolbar {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
h3 {
  font-size: 16px;
  font-weight: 600;
  color: #fafafa;
  margin: 0;
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
:deep(.n-button--primary-type) {
  --n-color: #6366f1;
  --n-color-hover: #818cf8;
  --n-color-pressed: #4f46e5;
  border-radius: 8px;
}
</style>
