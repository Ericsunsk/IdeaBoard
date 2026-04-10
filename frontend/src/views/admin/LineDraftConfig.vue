<template>
  <div class="line-draft-page">
    <div class="toolbar">
      <span style="font-size: 16px; font-weight: 600; color: #e4e4e7;">线稿模式管理</span>
      <n-button type="primary" @click="openAddModal">
        <template #icon>
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>
        </template>
        新增模式
      </n-button>
    </div>

    <div class="table-card">
      <n-data-table
        :columns="columns"
        :data="modes"
        :loading="loading"
        :pagination="false"
        :bordered="false"
        :row-key="(row: any) => row.id"
      />
    </div>

    <!-- 新增/编辑弹窗 -->
    <n-modal v-model:show="showModal" preset="card" :title="isEditing ? '编辑模式' : '新增模式'" :style="{ maxWidth: '560px' }">
      <n-form ref="formRef" :model="formData" label-placement="top" size="large">
        <n-form-item label="模式名称" path="name">
          <n-input v-model:value="formData.name" placeholder="如：黑白线稿、彩色线稿" />
        </n-form-item>

        <n-form-item label="封面图片" path="cover_image">
          <div class="cover-upload-area">
            <div class="cover-preview">
              <img v-if="formData.cover_image" :src="formData.cover_image" alt="封面预览" @error="e => (e.target as HTMLImageElement).style.display='none'" />
              <div v-else class="cover-placeholder">暂无封面</div>
            </div>
            <n-upload
              :action="adminUploadAction"
              :headers="uploadHeaders"
              :max="1"
              :show-file-list="false"
              :accept="IMAGE_UPLOAD_ACCEPT_EXTENDED"
              @finish="handleUploadFinish"
              @before-upload="() => { uploading = true; return true }"
            >
              <n-button :loading="uploading" secondary>
                {{ formData.cover_image ? '重新上传' : '上传封面' }}
              </n-button>
            </n-upload>
          </div>
        </n-form-item>

        <n-form-item label="提示词模板（发送给 API）" path="prompt">
          <n-input
            v-model:value="formData.prompt"
            type="textarea"
            placeholder="用户不可见，提交任务时自动拼接到 prompt"
            :rows="3"
          />
          <template #feedback>
            <span style="color: #71717a; font-size: 12px;">用户选择此模式后，系统将此提示词与上传图片一起发送给 API</span>
          </template>
        </n-form-item>

        <n-grid :cols="2" :x-gap="16">
          <n-gi>
            <n-form-item label="排序权重" path="sort_order">
              <n-input-number v-model:value="formData.sort_order" :min="0" :max="9999" style="width: 100%" />
            </n-form-item>
          </n-gi>
          <n-gi>
            <n-form-item label="是否启用">
              <n-switch v-model:value="formData.is_enabled">
                <template #checked>启用</template>
                <template #unchecked>禁用</template>
              </n-switch>
            </n-form-item>
          </n-gi>
        </n-grid>
      </n-form>

      <template #footer>
        <div class="modal-footer">
          <n-button @click="showModal = false">取消</n-button>
          <n-button type="primary" :loading="saving" @click="handleSave">保存</n-button>
        </div>
      </template>
    </n-modal>

    <!-- 删除确认 -->
    <n-modal v-model:show="showDeleteModal" preset="card" title="确认删除" :style="{ maxWidth: '400px' }">
      <p style="color: #a1a1aa; line-height: 1.8;">确定要删除「{{ deleteTarget?.name }}」吗？此操作不可撤销。</p>
      <template #footer>
        <div class="modal-footer">
          <n-button @click="showDeleteModal = false">取消</n-button>
          <n-button type="error" :loading="deleting" @click="confirmDelete">删除</n-button>
        </div>
      </template>
    </n-modal>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, h } from 'vue'
import {
  NDataTable, NButton, NInput, NModal, NForm, NFormItem,
  NSwitch, NGrid, NGi, NInputNumber, NUpload, useMessage
} from 'naive-ui'
import type { UploadFileInfo } from 'naive-ui'
import { lineDraftApi } from '../../api/index'
import { IMAGE_UPLOAD_ACCEPT_EXTENDED, UPLOAD_FAILED_MESSAGE, UPLOAD_SUCCESS_MESSAGE } from '../../constants/media'
import { adminUploadAction, parseAdminUploadResponse, useAdminUploadHeaders } from '../../utils/admin-upload'

const message = useMessage()

const modes = ref<any[]>([])
const loading = ref(false)

async function loadModes() {
  loading.value = true
  try {
    const res: any = await lineDraftApi.adminList()
    if (res.code === 0) modes.value = res.data.list || []
  } finally {
    loading.value = false
  }
}

onMounted(loadModes)

const columns = [
  {
    title: '封面',
    key: 'cover_image',
    width: 100,
    render(row: any) {
      return h('div', {
        style: 'width: 70px; height: 50px; border-radius: 6px; overflow: hidden; background: #27272a;'
      }, [
        row.cover_image ? h('img', {
          src: row.cover_image,
          style: 'width: 100%; height: 100%; object-fit: cover;',
          onError: (e: Event) => { (e.target as HTMLImageElement).style.display = 'none' }
        }) : null
      ])
    }
  },
  { title: '名称', key: 'name', ellipsis: { tooltip: true } },
  { title: '提示词', key: 'prompt', ellipsis: { tooltip: true } },
  {
    title: '排序', key: 'sort_order', width: 70,
    render(row: any) { return h('span', { style: 'color: #71717a;' }, row.sort_order) }
  },
  {
    title: '状态', key: 'is_enabled', width: 80,
    render(row: any) {
      return h('span', {
        style: row.is_enabled ? 'color: #22c55e;' : 'color: #ef4444;'
      }, row.is_enabled ? '启用' : '禁用')
    }
  },
  {
    title: '操作', key: 'actions', width: 140,
    render(row: any) {
      return h('div', { style: 'display: flex; gap: 8px;' }, [
        h(NButton, { size: 'small', secondary: true, onClick: () => openEditModal(row) }, { default: () => '编辑' }),
        h(NButton, { size: 'small', tertiary: true, style: 'color: #ef4444;', onClick: () => openDeleteModal(row) }, { default: () => '删除' })
      ])
    }
  }
]

// ============ 新增/编辑 ============
const showModal = ref(false)
const isEditing = ref(false)
const saving = ref(false)
const formRef = ref()
const formData = ref({ name: '', cover_image: '', prompt: '', sort_order: 0, is_enabled: true })
const editingId = ref<number | null>(null)
const uploading = ref(false)

const uploadHeaders = useAdminUploadHeaders()

function openAddModal() {
  isEditing.value = false
  editingId.value = null
  formData.value = { name: '', cover_image: '', prompt: '', sort_order: 0, is_enabled: true }
  showModal.value = true
}

function openEditModal(row: any) {
  isEditing.value = true
  editingId.value = row.id
  formData.value = {
    name: row.name,
    cover_image: row.cover_image,
    prompt: row.prompt,
    sort_order: row.sort_order,
    is_enabled: row.is_enabled,
  }
  showModal.value = true
}

function handleUploadFinish({ file, event }: { file: UploadFileInfo; event?: ProgressEvent }) {
  uploading.value = false
  const res = parseAdminUploadResponse(event)
  if (res.code === 0 && res.data?.url) {
    formData.value.cover_image = res.data.url
    message.success(UPLOAD_SUCCESS_MESSAGE)
  } else {
    message.error(res.msg || UPLOAD_FAILED_MESSAGE)
  }
  return file
}

async function handleSave() {
  if (!formData.value.name.trim()) {
    message.warning('请输入模式名称')
    return
  }
  if (!formData.value.prompt.trim()) {
    message.warning('请输入提示词模板')
    return
  }
  saving.value = true
  try {
    if (isEditing.value && editingId.value) {
      const res: any = await lineDraftApi.adminUpdate(editingId.value, formData.value)
      if (res.code === 0) {
        message.success('更新成功')
        showModal.value = false
        loadModes()
      } else {
        message.error(res.msg || '更新失败')
      }
    } else {
      const res: any = await lineDraftApi.adminCreate(formData.value)
      if (res.code === 0) {
        message.success('创建成功')
        showModal.value = false
        loadModes()
      } else {
        message.error(res.msg || '创建失败')
      }
    }
  } finally {
    saving.value = false
  }
}

// ============ 删除 ============
const showDeleteModal = ref(false)
const deleting = ref(false)
const deleteTarget = ref<any>(null)

function openDeleteModal(row: any) {
  deleteTarget.value = row
  showDeleteModal.value = true
}

async function confirmDelete() {
  if (!deleteTarget.value) return
  deleting.value = true
  try {
    const res: any = await lineDraftApi.adminDelete(deleteTarget.value.id)
    if (res.code === 0) {
      message.success('已删除')
      showDeleteModal.value = false
      loadModes()
    } else {
      message.error(res.msg || '删除失败')
    }
  } finally {
    deleting.value = false
  }
}
</script>

<style scoped>
@import './admin-common.css';

.line-draft-page {
  padding: 28px 32px;
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.table-card {
  background: #18181b;
  border-radius: 12px;
  padding: 16px;
}

.cover-upload-area {
  display: flex;
  align-items: center;
  gap: 16px;
}

.cover-preview {
  width: 80px;
  height: 56px;
  border-radius: 8px;
  overflow: hidden;
  background: #27272a;
  display: flex;
  align-items: center;
  justify-content: center;
}

.cover-preview img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.cover-placeholder {
  font-size: 12px;
  color: #52525b;
}

.modal-footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}
</style>
