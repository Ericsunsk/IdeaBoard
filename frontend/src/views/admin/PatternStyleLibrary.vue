<template>
  <div class="face-library-page">
    <div class="toolbar">
      <n-select v-model:value="filterCategory" :options="categoryOptions" placeholder="工作流分类" clearable style="width: 180px" />
      <n-select v-model:value="filterEnabled" :options="statusOptions" placeholder="上下架状态" clearable style="width: 140px" />
      <n-button @click="loadList">
        <template #icon><svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="23 4 23 10 17 10"/><polyline points="1 20 1 14 7 14"/><path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15"/></svg></template>
        刷新
      </n-button>
      <n-button type="primary" @click="openAddModal">
        <template #icon><svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg></template>
        新增风格模型
      </n-button>
    </div>

    <div class="table-card">
      <n-data-table :columns="columns" :data="filteredList" :loading="loading" :pagination="false" :bordered="false" :row-key="(row: any) => row.id" />
      <div class="pagination-wrapper">
        <n-pagination v-model:page="page" :page-count="pageCount" @update:page="loadList" />
      </div>
    </div>

    <n-modal v-model:show="showModal" preset="card" :title="isEditing ? '编辑风格模型' : '新增风格模型'" :style="{ maxWidth: '520px' }">
      <n-form ref="formRef" :model="formData" :rules="rules" label-placement="top" size="large">
        <n-form-item label="名称" path="name">
          <n-input v-model:value="formData.name" placeholder="如：水墨国画、波普艺术" />
        </n-form-item>
        <n-form-item label="工作流分类" path="category">
          <n-select v-model:value="formData.category" :options="categoryFormOptions" placeholder="选择所属工作流" />
        </n-form-item>
        <n-form-item label="风格预览图" path="image_url">
          <div class="cover-upload-area">
            <div class="cover-preview square">
              <img v-if="formData.image_url" :src="formData.image_url" alt="预览" @error="e => (e.target as HTMLImageElement).style.display='none'" />
              <div v-else class="cover-placeholder">暂无图片</div>
            </div>
            <n-upload :action="adminUploadAction" :headers="uploadHeaders" :max="1" :show-file-list="false" :accept="IMAGE_UPLOAD_ACCEPT" @finish="handleUploadFinish" @before-upload="() => { uploading = true; return true }">
              <n-button :loading="uploading" secondary>
                <template #icon><svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="17 8 12 3 7 8"/><line x1="12" y1="3" x2="12" y2="15"/></svg></template>
                {{ formData.image_url ? '重新上传' : '上传图片' }}
              </n-button>
            </n-upload>
          </div>
        </n-form-item>
        <n-grid :cols="2" :x-gap="16">
          <n-gi><n-form-item label="排序权重" path="sort_order"><n-input-number v-model:value="formData.sort_order" :min="0" :max="9999" style="width: 100%" /></n-form-item></n-gi>
          <n-gi><n-form-item label="是否上架" path="is_enabled"><n-switch v-model:value="formData.is_enabled"><template #checked>上架</template><template #unchecked>下架</template></n-switch></n-form-item></n-gi>
        </n-grid>
      </n-form>
      <template #footer><div class="modal-footer"><n-button @click="showModal = false">取消</n-button><n-button type="primary" :loading="saving" @click="handleSave">保存</n-button></div></template>
    </n-modal>

    <n-modal v-model:show="showDeleteModal" preset="card" title="确认删除" :style="{ maxWidth: '400px' }">
      <p style="color: #a1a1aa; line-height: 1.8;">确定要删除风格模型「{{ deleteTarget?.name }}」吗？此操作不可撤销。</p>
      <template #footer><div class="modal-footer"><n-button @click="showDeleteModal = false">取消</n-button><n-button type="error" :loading="deleting" @click="confirmDelete">删除</n-button></div></template>
    </n-modal>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, h } from 'vue'
import { NDataTable, NPagination, NButton, NSelect, NModal, NForm, NFormItem, NInput, NSwitch, NGrid, NGi, NInputNumber, NUpload, useMessage } from 'naive-ui'
import type { UploadFileInfo } from 'naive-ui'
import { patternStyleApi } from '../../api/index'
import { IMAGE_UPLOAD_ACCEPT, UPLOAD_FAILED_MESSAGE, UPLOAD_SUCCESS_MESSAGE } from '../../constants/media'
import { adminUploadAction, parseAdminUploadResponse, useAdminUploadHeaders } from '../../utils/admin-upload'

const message = useMessage()

interface Item { id: number; name: string; image_url: string; category: string; sort_order: number; is_enabled: boolean }
const list = ref<Item[]>([])
const loading = ref(false)
const page = ref(1)
const pageSize = 50
const pageCount = computed(() => Math.max(1, Math.ceil(list.value.length / pageSize)))

const filterCategory = ref<string | null>(null)
const filterEnabled = ref<number | null>(null)

const categoryOptions = [
  { label: 'Generate · 图案生成', value: 'pattern-gen' },
  { label: 'Generate · 画风复刻', value: 'style-clone' },
]
const statusOptions = [{ label: '已上架', value: 1 }, { label: '已下架', value: 0 }]
const archivedCategoryLabelMap: Record<string, string> = {
  'seamless-tile': 'Archived · 四方连续',
  'pattern-fusion': 'Archived · 图案融合',
  'craft-fusion': 'Archived · 工艺融合',
  'pattern-inpaint': 'Archived · 图案涂改',
  'pattern-upscale': 'Archived · 图案变清晰',
}
const categoryLabelMap: Record<string, string> = {
  ...Object.fromEntries(categoryOptions.map(o => [o.value, o.label])),
  ...archivedCategoryLabelMap,
}
const categoryFormOptions = computed(() => {
  const currentValue = formData.value.category
  if (!currentValue || categoryOptions.some(option => option.value === currentValue)) {
    return categoryOptions
  }
  return [
    { label: archivedCategoryLabelMap[currentValue] || `Archived · ${currentValue}`, value: currentValue, disabled: true },
    ...categoryOptions,
  ]
})

const filteredList = computed(() => {
  let result = list.value
  if (filterCategory.value) result = result.filter(f => f.category === filterCategory.value)
  if (filterEnabled.value !== null) result = result.filter(f => f.is_enabled === (filterEnabled.value === 1))
  return result
})

const columns = [
  {
    title: '预览', key: 'image_url', width: 70,
    render(row: Item) {
      return h('div', { style: 'width: 50px; height: 50px; border-radius: 8px; overflow: hidden; background: #27272a;' }, [
        h('img', { src: row.image_url, style: 'width: 100%; height: 100%; object-fit: cover;', onError: (e: Event) => { (e.target as HTMLImageElement).style.display = 'none' } })
      ])
    }
  },
  { title: '名称', key: 'name', ellipsis: { tooltip: true } },
  { title: '工作流分类', key: 'category', width: 160, render(row: Item) { return h('span', { style: 'color: #a1a1aa; font-size: 12px;' }, categoryLabelMap[row.category] || row.category) } },
  { title: '排序', key: 'sort_order', width: 70, render(row: Item) { return h('span', { style: 'color: #71717a;' }, row.sort_order) } },
  { title: '状态', key: 'is_enabled', width: 80, render(row: Item) { return h('span', { style: row.is_enabled ? 'color: #22c55e;' : 'color: #ef4444;' }, row.is_enabled ? '已上架' : '已下架') } },
  {
    title: '操作', key: 'actions', width: 140,
    render(row: Item) {
      return h('div', { style: 'display: flex; gap: 8px;' }, [
        h(NButton, { size: 'small', secondary: true, onClick: () => openEditModal(row) }, { default: () => '编辑' }),
        h(NButton, { size: 'small', tertiary: true, style: 'color: #ef4444;', onClick: () => openDeleteModal(row) }, { default: () => '删除' }),
      ])
    }
  },
]

const showModal = ref(false)
const isEditing = ref(false)
const saving = ref(false)
const formRef = ref()
const formData = ref({ name: '', image_url: '', category: 'pattern-gen', sort_order: 0, is_enabled: true })
const editingId = ref<number | null>(null)
const uploading = ref(false)

const uploadHeaders = useAdminUploadHeaders()

function handleUploadFinish({ file, event }: { file: UploadFileInfo; event?: ProgressEvent }) {
  uploading.value = false
  const res = parseAdminUploadResponse(event)
  if (res.code === 0 && res.data?.url) {
    formData.value.image_url = res.data.url
    message.success(UPLOAD_SUCCESS_MESSAGE)
  } else {
    message.error(res.msg || UPLOAD_FAILED_MESSAGE)
  }
  return file
}

const rules = {
  name: [{ required: true, message: '请输入名称', trigger: 'blur' }],
  image_url: [{ required: true, message: '请上传预览图', trigger: 'change' }],
  category: [{ required: true, message: '请选择功能分类', trigger: 'change' }],
}

function openAddModal() { isEditing.value = false; editingId.value = null; formData.value = { name: '', image_url: '', category: 'pattern-gen', sort_order: 0, is_enabled: true }; showModal.value = true }
function openEditModal(item: Item) { isEditing.value = true; editingId.value = item.id; formData.value = { name: item.name, image_url: item.image_url, category: item.category, sort_order: item.sort_order, is_enabled: item.is_enabled }; showModal.value = true }

async function handleSave() {
  try { await formRef.value?.validate() } catch { return }
  saving.value = true
  try {
    if (isEditing.value && editingId.value) { await patternStyleApi.adminUpdate(editingId.value, formData.value); message.success('更新成功') }
    else { await patternStyleApi.adminCreate(formData.value); message.success('创建成功') }
    showModal.value = false; await loadList()
  } catch (e: any) { message.error(e.msg || '操作失败') }
  finally { saving.value = false }
}

const showDeleteModal = ref(false)
const deleting = ref(false)
const deleteTarget = ref<Item | null>(null)
function openDeleteModal(item: Item) { deleteTarget.value = item; showDeleteModal.value = true }
async function confirmDelete() {
  if (!deleteTarget.value) return
  deleting.value = true
  try { await patternStyleApi.adminDelete(deleteTarget.value.id); message.success('删除成功'); showDeleteModal.value = false; await loadList() }
  catch (e: any) { message.error(e.msg || '删除失败') }
  finally { deleting.value = false }
}

async function loadList() {
  loading.value = true
  try { const res: any = await patternStyleApi.adminList({ page: 1, page_size: 200 }); if (res.code === 0) list.value = res.data?.list || [] }
  catch { message.error('加载失败') }
  finally { loading.value = false }
}

onMounted(() => { loadList() })
</script>

<style scoped>
@import './admin-common.css';
.face-library-page { padding: 28px 32px; display: flex; flex-direction: column; gap: 20px; max-width: 1400px; }
.toolbar { display: flex; align-items: center; gap: 12px; flex-wrap: wrap; }
.table-card { background: #18181b; border-radius: 12px; border: 1px solid #27272a; padding: 16px; }
.pagination-wrapper { margin-top: 16px; display: flex; justify-content: flex-end; }
.cover-upload-area { display: flex; align-items: center; gap: 16px; }
.cover-preview.square { width: 80px; height: 80px; border-radius: 8px; overflow: hidden; background: #27272a; border: 1px solid #3f3f46; flex-shrink: 0; }
.cover-preview img { width: 100%; height: 100%; object-fit: cover; }
.cover-placeholder { width: 100%; height: 100%; display: flex; align-items: center; justify-content: center; font-size: 12px; color: #52525b; }
.modal-footer { display: flex; justify-content: flex-end; gap: 8px; }
:deep(.n-input) { --n-border: 1px solid #27272a; --n-border-hover: 1px solid #6366f1; --n-border-focus: 1px solid #6366f1; --n-color: #18181b; --n-color-focus: #18181b; --n-text-color: #e4e4e7; --n-placeholder-color: #52525b; border-radius: 8px; }
:deep(.n-button--primary-type) { --n-color: #6366f1; --n-color-hover: #818cf8; --n-color-pressed: #4f46e5; border-radius: 8px; }
:deep(.n-button--error-type) { --n-color: #ef4444; --n-color-hover: #f87171; border-radius: 8px; }
:deep(.n-data-table) { --n-td-color: transparent; --n-th-color: transparent; --n-border-color: #27272a; --n-td-color-hover: rgba(99, 102, 241, 0.05); --n-th-text-color: #71717a; --n-td-text-color: #e4e4e7; }
</style>
