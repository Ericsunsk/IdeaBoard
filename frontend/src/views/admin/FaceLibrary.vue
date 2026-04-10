<template>
  <div class="face-library-page">
    <!-- 工具栏 -->
    <div class="toolbar">
      <n-select
        v-model:value="filterCategory"
        :options="categoryOptions"
        placeholder="分类筛选"
        clearable
        style="width: 140px"
      />
      <n-select
        v-model:value="filterEnabled"
        :options="statusOptions"
        placeholder="上下架状态"
        clearable
        style="width: 140px"
      />
      <n-button @click="loadFaces">
        <template #icon>
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="23 4 23 10 17 10"/><polyline points="1 20 1 14 7 14"/><path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15"/></svg>
        </template>
        刷新
      </n-button>
      <n-button type="primary" @click="openAddModal">
        <template #icon>
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>
        </template>
        新增人脸
      </n-button>
    </div>

    <!-- 列表 -->
    <div class="table-card">
      <n-data-table
        :columns="columns"
        :data="filteredFaces"
        :loading="loading"
        :pagination="false"
        :bordered="false"
        :row-key="(row: any) => row.id"
      />
      <div class="pagination-wrapper">
        <n-pagination v-model:page="page" :page-count="pageCount" @update:page="loadFaces" />
      </div>
    </div>

    <!-- 新增/编辑弹窗 -->
    <n-modal v-model:show="showModal" preset="card" :title="isEditing ? '编辑人脸' : '新增人脸'" :style="{ maxWidth: '520px' }">
      <n-form ref="formRef" :model="formData" :rules="rules" label-placement="top" size="large">
        <n-form-item label="名称" path="name">
          <n-input v-model:value="formData.name" placeholder="如：模特A、男模1" />
        </n-form-item>

        <n-form-item label="分类" path="category">
          <n-select v-model:value="formData.category" :options="[{ label: '男人', value: 'male' }, { label: '女人', value: 'female' }]" placeholder="选择分类" />
        </n-form-item>

        <n-form-item label="人脸图片" path="image_url">
          <div class="cover-upload-area">
            <div class="cover-preview">
              <img v-if="formData.image_url" :src="formData.image_url" alt="人脸预览" @error="e => (e.target as HTMLImageElement).style.display='none'" />
              <div v-else class="cover-placeholder">暂无图片</div>
            </div>
            <n-upload
              :action="'/api/admin/upload'"
              :headers="uploadHeaders"
              :max="1"
              :show-file-list="false"
              accept="image/jpeg,image/png"
              @finish="handleUploadFinish"
              @before-upload="() => { uploading = true; return true }"
            >
              <n-button :loading="uploading" secondary>
                <template #icon>
                  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="17 8 12 3 7 8"/><line x1="12" y1="3" x2="12" y2="15"/></svg>
                </template>
                {{ formData.image_url ? '重新上传' : '上传图片' }}
              </n-button>
            </n-upload>
          </div>
        </n-form-item>

        <n-grid :cols="2" :x-gap="16">
          <n-gi>
            <n-form-item label="排序权重" path="sort_order">
              <n-input-number v-model:value="formData.sort_order" :min="0" :max="9999" style="width: 100%" />
            </n-form-item>
          </n-gi>
          <n-gi>
            <n-form-item label="是否上架" path="is_enabled">
              <n-switch v-model:value="formData.is_enabled">
                <template #checked>上架</template>
                <template #unchecked>下架</template>
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

    <!-- 删除确认弹窗 -->
    <n-modal v-model:show="showDeleteModal" preset="card" title="确认删除" :style="{ maxWidth: '400px' }">
      <p style="color: #a1a1aa; line-height: 1.8;">确定要删除人脸「{{ deleteTarget?.name }}」吗？此操作不可撤销。</p>
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
  NDataTable, NPagination, NButton, NSelect, NModal, NForm, NFormItem,
  NInput, NSwitch, NGrid, NGi, NInputNumber, NUpload, useMessage
} from 'naive-ui'
import type { UploadFileInfo } from 'naive-ui'
import { faceTemplateApi } from '../../api/index'

const message = useMessage()

// ============ 列表数据 ============
interface FaceItem { id: number; name: string; image_url: string; category: string; sort_order: number; is_enabled: boolean }
const faces = ref<FaceItem[]>([])
const loading = ref(false)
const page = ref(1)
const pageSize = 50
const pageCount = computed(() => Math.max(1, Math.ceil(faces.value.length / pageSize)))

const filterCategory = ref<string | null>(null)
const filterEnabled = ref<number | null>(null)

const categoryOptions = [
  { label: '男人', value: 'male' },
  { label: '女人', value: 'female' },
]
const statusOptions = [
  { label: '已上架', value: 1 },
  { label: '已下架', value: 0 },
]

const filteredFaces = computed(() => {
  let result = faces.value
  if (filterCategory.value) result = result.filter(f => f.category === filterCategory.value)
  if (filterEnabled.value !== null) result = result.filter(f => f.is_enabled === (filterEnabled.value === 1))
  return result
})

// ============ 表格列定义 ============
const columns = [
  {
    title: '图片', key: 'image_url', width: 80,
    render(row: FaceItem) {
      return h('div', { style: 'width: 50px; height: 66px; border-radius: 8px; overflow: hidden; background: #27272a;' }, [
        h('img', { src: row.image_url, style: 'width: 100%; height: 100%; object-fit: cover;', onError: (e: Event) => { (e.target as HTMLImageElement).style.display = 'none' } })
      ])
    }
  },
  { title: '名称', key: 'name', ellipsis: { tooltip: true } },
  {
    title: '分类', key: 'category', width: 80,
    render(row: FaceItem) { return h('span', { style: row.category === 'male' ? 'color: #60a5fa;' : 'color: #f472b6;' }, row.category === 'male' ? '男人' : '女人') }
  },
  {
    title: '排序', key: 'sort_order', width: 70,
    render(row: FaceItem) { return h('span', { style: 'color: #71717a;' }, row.sort_order) }
  },
  {
    title: '状态', key: 'is_enabled', width: 80,
    render(row: FaceItem) { return h('span', { style: row.is_enabled ? 'color: #22c55e;' : 'color: #ef4444;' }, row.is_enabled ? '已上架' : '已下架') }
  },
  {
    title: '操作', key: 'actions', width: 140,
    render(row: FaceItem) {
      return h('div', { style: 'display: flex; gap: 8px;' }, [
        h(NButton, { size: 'small', secondary: true, onClick: () => openEditModal(row) }, { default: () => '编辑' }),
        h(NButton, { size: 'small', tertiary: true, style: 'color: #ef4444;', onClick: () => openDeleteModal(row) }, { default: () => '删除' }),
      ])
    }
  },
]

// ============ 新增/编辑 ============
const showModal = ref(false)
const isEditing = ref(false)
const saving = ref(false)
const formRef = ref()
const formData = ref({ name: '', image_url: '', category: 'male', sort_order: 0, is_enabled: true })
const editingId = ref<number | null>(null)
const uploading = ref(false)

const uploadHeaders = computed(() => ({ Authorization: 'Bearer ' + (localStorage.getItem('token') || '') }))

function handleUploadFinish({ file, event }: { file: UploadFileInfo; event?: ProgressEvent }) {
  uploading.value = false
  try {
    const res = JSON.parse((event?.target as XMLHttpRequest)?.response || '{}')
    if (res.code === 0 && res.data?.url) { formData.value.image_url = res.data.url; message.success('上传成功') }
    else { message.error(res.msg || '上传失败') }
  } catch { message.error('上传失败') }
  return file
}

const rules = {
  name: [{ required: true, message: '请输入名称', trigger: 'blur' }],
  image_url: [{ required: true, message: '请上传人脸图片', trigger: 'change' }],
  category: [{ required: true, message: '请选择分类', trigger: 'change' }],
}

function openAddModal() {
  isEditing.value = false; editingId.value = null
  formData.value = { name: '', image_url: '', category: 'male', sort_order: 0, is_enabled: true }
  showModal.value = true
}

function openEditModal(face: FaceItem) {
  isEditing.value = true; editingId.value = face.id
  formData.value = { name: face.name, image_url: face.image_url, category: face.category, sort_order: face.sort_order, is_enabled: face.is_enabled }
  showModal.value = true
}

async function handleSave() {
  try { await formRef.value?.validate() } catch { return }
  saving.value = true
  try {
    if (isEditing.value && editingId.value) {
      await faceTemplateApi.adminUpdate(editingId.value, formData.value)
      message.success('更新成功')
    } else {
      await faceTemplateApi.adminCreate(formData.value)
      message.success('创建成功')
    }
    showModal.value = false
    await loadFaces()
  } catch (e: any) { message.error(e.msg || '操作失败') }
  finally { saving.value = false }
}

// ============ 删除 ============
const showDeleteModal = ref(false)
const deleting = ref(false)
const deleteTarget = ref<FaceItem | null>(null)

function openDeleteModal(face: FaceItem) { deleteTarget.value = face; showDeleteModal.value = true }

async function confirmDelete() {
  if (!deleteTarget.value) return
  deleting.value = true
  try {
    await faceTemplateApi.adminDelete(deleteTarget.value.id)
    message.success('删除成功'); showDeleteModal.value = false; await loadFaces()
  } catch (e: any) { message.error(e.msg || '删除失败') }
  finally { deleting.value = false }
}

// ============ 加载数据 ============
async function loadFaces() {
  loading.value = true
  try {
    const res: any = await faceTemplateApi.adminList({ page: 1, page_size: 200 })
    if (res.code === 0) faces.value = res.data?.list || []
  } catch { message.error('加载失败') }
  finally { loading.value = false }
}

onMounted(() => { loadFaces() })
</script>

<style scoped>
@import './admin-common.css';

.face-library-page {
  padding: 28px 32px;
  display: flex;
  flex-direction: column;
  gap: 20px;
  max-width: 1400px;
}
.toolbar { display: flex; align-items: center; gap: 12px; flex-wrap: wrap; }
.table-card { background: #18181b; border-radius: 12px; border: 1px solid #27272a; padding: 16px; }
.pagination-wrapper { margin-top: 16px; display: flex; justify-content: flex-end; }
.cover-upload-area { display: flex; align-items: center; gap: 16px; }
.cover-preview { width: 80px; height: 106px; border-radius: 8px; overflow: hidden; background: #27272a; border: 1px solid #3f3f46; flex-shrink: 0; }
.cover-preview img { width: 100%; height: 100%; object-fit: cover; }
.cover-placeholder { width: 100%; height: 100%; display: flex; align-items: center; justify-content: center; font-size: 12px; color: #52525b; }
.modal-footer { display: flex; justify-content: flex-end; gap: 8px; }

:deep(.n-input) { --n-border: 1px solid #27272a; --n-border-hover: 1px solid #6366f1; --n-border-focus: 1px solid #6366f1; --n-color: #18181b; --n-color-focus: #18181b; --n-text-color: #e4e4e7; --n-placeholder-color: #52525b; border-radius: 8px; }
:deep(.n-button--primary-type) { --n-color: #6366f1; --n-color-hover: #818cf8; --n-color-pressed: #4f46e5; border-radius: 8px; }
:deep(.n-button--error-type) { --n-color: #ef4444; --n-color-hover: #f87171; border-radius: 8px; }
:deep(.n-data-table) { --n-td-color: transparent; --n-th-color: transparent; --n-border-color: #27272a; --n-td-color-hover: rgba(99, 102, 241, 0.05); --n-th-text-color: #71717a; --n-td-text-color: #e4e4e7; }
</style>
