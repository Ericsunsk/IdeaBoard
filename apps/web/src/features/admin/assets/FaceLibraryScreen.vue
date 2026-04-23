<template>
  <div class="admin-page face-library-page">
    <AdminAssetPageLayout
      title="人脸素材库"
      description="维护人脸模板素材与分类配置，支持排序和上下架，变更实时作用于素材选择面板。"
      :total="faces.length"
      table-title="人脸列表"
      table-description="支持编辑、上下架与删除操作。分类用于工作流人脸维度筛选。"
    >
      <template #summary>
        <AdminSummaryCard label="人脸总数" :value="faces.length" foot="含下架素材" />
        <AdminSummaryCard label="已上架" :value="enabledFaceCount" foot="用户可见" />
        <AdminSummaryCard label="男性分类" :value="maleFaceCount" foot="male 分类" />
        <AdminSummaryCard label="筛选结果" :value="filteredFaces.length" foot="当前列表" />
      </template>
      <template #toolbar>
        <span class="toolbar-group-title">筛选</span>
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
      </template>
      <template #table>
        <n-data-table
          :columns="columns"
          :data="filteredFaces"
          :loading="loading"
          :pagination="false"
          :bordered="false"
          :row-key="(row: FaceTemplate) => row.id"
        />
      </template>
      <template #modals>

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
          <AdminImageUploadField
            :image-url="formData.image_url"
            empty-text="暂无图片"
            alt="人脸预览"
            :accept="IMAGE_UPLOAD_ACCEPT"
            :loading="uploading"
            :action="adminUploadAction"
            :headers="uploadHeaders"
            preview-variant="portrait"
            @finish="handleUploadFinish"
            @start-upload="uploading = true"
          />
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
        <p class="admin-modal-text">确定要删除人脸「{{ deleteTarget?.name }}」吗？此操作不可撤销。</p>
        <template #footer>
          <div class="modal-footer">
            <n-button @click="showDeleteModal = false">取消</n-button>
            <n-button type="error" :loading="deleting" @click="confirmDelete">删除</n-button>
          </div>
        </template>
      </n-modal>
      </template>
    </AdminAssetPageLayout>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, h } from 'vue'
import {
  NDataTable, NButton, NSelect, NModal, NForm, NFormItem,
  NInput, NSwitch, NGrid, NGi, NInputNumber, useMessage
} from 'naive-ui'
import { faceTemplateApi } from '../../../shared/api/index'
import type { FaceTemplate, FaceTemplateFormData } from '../../../shared/api/index'
import { IMAGE_UPLOAD_ACCEPT } from '../../../shared/constants/media'
import { adminUploadAction, useAdminUploadHeaders } from '../shared/adminUpload'
import {
  ADMIN_ASSET_LIST_QUERY,
  ADMIN_STATUS_OPTIONS,
  createAdminUploadFinishHandler,
  useAdminCrudModal,
  useAdminDeleteModal,
  useAdminListState,
} from '../shared/assetLibraryShared'
import AdminAssetPageLayout from '../shared/components/AdminAssetPageLayout.vue'
import AdminImageUploadField from '../shared/components/AdminImageUploadField.vue'
import AdminSummaryCard from '../shared/components/AdminSummaryCard.vue'

const message = useMessage()

// ============ 列表数据 ============
const { list: faces, loading, loadList: loadFaces } = useAdminListState<FaceTemplate>({
  fetcher: () => faceTemplateApi.adminList(ADMIN_ASSET_LIST_QUERY),
  message,
  errorMessage: '加载失败',
})

const filterCategory = ref<string | null>(null)
const filterEnabled = ref<number | null>(null)

const categoryOptions = [
  { label: '男人', value: 'male' },
  { label: '女人', value: 'female' },
]
const statusOptions = ADMIN_STATUS_OPTIONS

const filteredFaces = computed(() => {
  let result = faces.value
  if (filterCategory.value) result = result.filter(f => f.category === filterCategory.value)
  if (filterEnabled.value !== null) result = result.filter(f => f.is_enabled === (filterEnabled.value === 1))
  return result
})
const enabledFaceCount = computed(() => faces.value.filter(item => item.is_enabled).length)
const maleFaceCount = computed(() => faces.value.filter(item => item.category === 'male').length)

// ============ 表格列定义 ============
const columns = [
  {
    title: '图片', key: 'image_url', width: 80,
    render(row: FaceTemplate) {
      return h('div', { style: 'width: 50px; height: 66px; border-radius: 8px; overflow: hidden; background: var(--admin-preview-bg); border: 1px solid var(--admin-border);' }, [
        h('img', { src: row.image_url, style: 'width: 100%; height: 100%; object-fit: cover;', onError: (e: Event) => { (e.target as HTMLImageElement).style.display = 'none' } })
      ])
    }
  },
  { title: '名称', key: 'name', ellipsis: { tooltip: true } },
  {
    title: '分类', key: 'category', width: 80,
    render(row: FaceTemplate) { return h('span', { style: 'color: var(--admin-text-2);' }, row.category === 'male' ? '男人' : '女人') }
  },
  {
    title: '排序', key: 'sort_order', width: 70,
    render(row: FaceTemplate) { return h('span', { style: 'color: var(--admin-text-3);' }, row.sort_order) }
  },
  {
    title: '状态', key: 'is_enabled', width: 80,
    render(row: FaceTemplate) { return h('span', { style: row.is_enabled ? 'color: var(--admin-success);' : 'color: var(--admin-danger);' }, row.is_enabled ? '已上架' : '已下架') }
  },
  {
    title: '操作', key: 'actions', width: 140,
    render(row: FaceTemplate) {
      return h('div', { style: 'display: flex; gap: 8px;' }, [
        h(NButton, { size: 'small', secondary: true, onClick: () => openEditModal(row) }, { default: () => '编辑' }),
        h(NButton, { size: 'small', tertiary: true, type: 'error', onClick: () => openDeleteModal(row) }, { default: () => '删除' }),
      ])
    }
  },
]

// ============ 新增/编辑 ============
const {
  showModal,
  isEditing,
  saving,
  formRef,
  formData,
  openAddModal,
  openEditModal,
  handleSave,
} = useAdminCrudModal<FaceTemplate, FaceTemplateFormData>({
  createDefault: () => ({ name: '', image_url: '', category: 'male', sort_order: 0, is_enabled: true }),
  toForm: (item) => ({ name: item.name, image_url: item.image_url, category: item.category, sort_order: item.sort_order, is_enabled: item.is_enabled }),
  create: (payload) => faceTemplateApi.adminCreate(payload),
  update: (id, payload) => faceTemplateApi.adminUpdate(id, payload),
  afterSave: loadFaces,
  message,
})
const uploading = ref(false)

const uploadHeaders = useAdminUploadHeaders()
const handleUploadFinish = createAdminUploadFinishHandler(uploading, formData, 'image_url', message)

const rules = {
  name: [{ required: true, message: '请输入名称', trigger: 'blur' }],
  image_url: [{ required: true, message: '请上传人脸图片', trigger: 'change' }],
  category: [{ required: true, message: '请选择分类', trigger: 'change' }],
}

// ============ 删除 ============
const { showDeleteModal, deleting, deleteTarget, openDeleteModal, confirmDelete } = useAdminDeleteModal<FaceTemplate>({
  deleteById: (id) => faceTemplateApi.adminDelete(id),
  afterDelete: loadFaces,
  message,
})

onMounted(() => { loadFaces() })
</script>

<style scoped>
@import '../shared/adminCommon.css';

@media (max-width: 720px) {
  .face-library-page :deep(.n-select),
  .face-library-page :deep(.n-button) {
    width: 100%;
  }
}
</style>
