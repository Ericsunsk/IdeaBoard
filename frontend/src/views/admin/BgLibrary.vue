<template>
  <div class="admin-page bg-library-page">
    <div class="page-header">
      <div>
        <h2 class="page-title">背景素材库</h2>
        <p class="page-desc">维护背景模板的分类、排序与上下架状态，变更会直接影响管理员可选素材。</p>
      </div>
      <div class="header-side-note">
        <span>总数 {{ list.length }}</span>
      </div>
    </div>

    <div class="summary-grid">
      <AdminSummaryCard label="背景总数" :value="list.length" foot="含下架素材" />
      <AdminSummaryCard label="已上架" :value="enabledCount" foot="用户可见" />
      <AdminSummaryCard label="场景类" :value="sceneCount" foot="scene 分类" />
      <AdminSummaryCard label="筛选结果" :value="filteredList.length" foot="当前列表" />
    </div>

    <div class="toolbar">
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
      <n-button @click="loadList">
        <template #icon>
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="23 4 23 10 17 10"/><polyline points="1 20 1 14 7 14"/><path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15"/></svg>
        </template>
        刷新
      </n-button>
      <n-button type="primary" @click="openAddModal">
        <template #icon>
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>
        </template>
        新增背景
      </n-button>
    </div>

    <div class="table-card">
      <AdminSectionHeader title="背景列表" description="支持编辑、上下架和删除操作。分类用于业务流程内筛选。" />
      <n-data-table
        :columns="columns"
        :data="filteredList"
        :loading="loading"
        :pagination="false"
        :bordered="false"
        :row-key="(row: BgTemplate) => row.id"
      />
      <div class="pagination-wrapper">
        <n-pagination v-model:page="page" :page-count="pageCount" @update:page="loadList" />
      </div>
    </div>

    <!-- 新增/编辑弹窗 -->
    <n-modal v-model:show="showModal" preset="card" :title="isEditing ? '编辑背景' : '新增背景'" :style="{ maxWidth: '520px' }">
      <n-form ref="formRef" :model="formData" :rules="rules" label-placement="top" size="large">
        <n-form-item label="名称" path="name">
          <n-input v-model:value="formData.name" placeholder="如：纯白背景、城市街道" />
        </n-form-item>

        <n-form-item label="分类" path="category">
          <n-select v-model:value="formData.category" :options="categoryFormOptions" placeholder="选择分类" />
        </n-form-item>

        <n-form-item label="背景图片" path="image_url">
          <div class="cover-upload-area">
            <div class="cover-preview landscape">
              <img v-if="formData.image_url" :src="formData.image_url" alt="背景预览" @error="e => (e.target as HTMLImageElement).style.display='none'" />
              <div v-else class="cover-placeholder">暂无图片</div>
            </div>
            <n-upload
              :action="adminUploadAction"
              :headers="uploadHeaders"
              :max="1"
              :show-file-list="false"
              :accept="IMAGE_UPLOAD_ACCEPT"
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
      <p style="color: #7884ad; line-height: 1.8;">确定要删除背景「{{ deleteTarget?.name }}」吗？此操作不可撤销。</p>
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
import { bgTemplateApi } from '../../api/index'
import type { BgTemplate, BgTemplateFormData } from '../../api/index'
import { IMAGE_UPLOAD_ACCEPT } from '../../constants/media'
import { adminUploadAction, useAdminUploadHeaders } from '../../utils/admin-upload'
import {
  ADMIN_ASSET_LIST_QUERY,
  ADMIN_ASSET_PAGE_SIZE,
  ADMIN_STATUS_OPTIONS,
  createAdminUploadFinishHandler,
  useAdminCrudModal,
  useAdminDeleteModal,
  useAdminListState,
} from './assetLibraryShared'
import AdminSummaryCard from './components/AdminSummaryCard.vue'
import AdminSectionHeader from './components/AdminSectionHeader.vue'

const message = useMessage()

// ============ 列表数据 ============
const { list, loading, loadList } = useAdminListState<BgTemplate>({
  fetcher: () => bgTemplateApi.adminList(ADMIN_ASSET_LIST_QUERY),
  message,
  errorMessage: '加载失败',
})
const page = ref(1)
const pageSize = ADMIN_ASSET_PAGE_SIZE
const pageCount = computed(() => Math.max(1, Math.ceil(list.value.length / pageSize)))

const filterCategory = ref<string | null>(null)
const filterEnabled = ref<number | null>(null)

const categoryOptions = [
  { label: '纯色', value: 'solid' },
  { label: '场景', value: 'scene' },
  { label: '自定义', value: 'custom' },
]
const categoryFormOptions = [
  { label: '纯色', value: 'solid' },
  { label: '场景', value: 'scene' },
  { label: '自定义', value: 'custom' },
]
const statusOptions = ADMIN_STATUS_OPTIONS

const categoryLabelMap: Record<string, string> = { solid: '纯色', scene: '场景', custom: '自定义' }

const filteredList = computed(() => {
  let result = list.value
  if (filterCategory.value) result = result.filter(f => f.category === filterCategory.value)
  if (filterEnabled.value !== null) result = result.filter(f => f.is_enabled === (filterEnabled.value === 1))
  return result
})
const enabledCount = computed(() => list.value.filter(item => item.is_enabled).length)
const sceneCount = computed(() => list.value.filter(item => item.category === 'scene').length)

// ============ 表格列定义 ============
const columns = [
  {
    title: '图片', key: 'image_url', width: 100,
    render(row: BgTemplate) {
      return h('div', { style: 'width: 70px; height: 50px; border-radius: 10px; overflow: hidden; background: rgba(113, 97, 239, 0.1);' }, [
        h('img', { src: row.image_url, style: 'width: 100%; height: 100%; object-fit: cover;', onError: (e: Event) => { (e.target as HTMLImageElement).style.display = 'none' } })
      ])
    }
  },
  { title: '名称', key: 'name', ellipsis: { tooltip: true } },
  {
    title: '分类', key: 'category', width: 80,
    render(row: BgTemplate) { return h('span', { style: 'color: #7c89b2;' }, categoryLabelMap[row.category] || row.category) }
  },
  {
    title: '排序', key: 'sort_order', width: 70,
    render(row: BgTemplate) { return h('span', { style: 'color: #8b95b8;' }, row.sort_order) }
  },
  {
    title: '状态', key: 'is_enabled', width: 80,
    render(row: BgTemplate) { return h('span', { style: row.is_enabled ? 'color: #1f9d68;' : 'color: #d65478;' }, row.is_enabled ? '已上架' : '已下架') }
  },
  {
    title: '操作', key: 'actions', width: 140,
    render(row: BgTemplate) {
      return h('div', { style: 'display: flex; gap: 8px;' }, [
        h(NButton, { size: 'small', secondary: true, onClick: () => openEditModal(row) }, { default: () => '编辑' }),
        h(NButton, { size: 'small', tertiary: true, style: 'color: #d65478;', onClick: () => openDeleteModal(row) }, { default: () => '删除' }),
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
} = useAdminCrudModal<BgTemplate, BgTemplateFormData>({
  createDefault: () => ({ name: '', image_url: '', category: 'solid', sort_order: 0, is_enabled: true }),
  toForm: (item) => ({ name: item.name, image_url: item.image_url, category: item.category, sort_order: item.sort_order, is_enabled: item.is_enabled }),
  create: (payload) => bgTemplateApi.adminCreate(payload),
  update: (id, payload) => bgTemplateApi.adminUpdate(id, payload),
  afterSave: loadList,
  message,
})
const uploading = ref(false)

const uploadHeaders = useAdminUploadHeaders()
const handleUploadFinish = createAdminUploadFinishHandler(uploading, formData, 'image_url', message)

const rules = {
  name: [{ required: true, message: '请输入名称', trigger: 'blur' }],
  image_url: [{ required: true, message: '请上传背景图片', trigger: 'change' }],
  category: [{ required: true, message: '请选择分类', trigger: 'change' }],
}

// ============ 删除 ============
const { showDeleteModal, deleting, deleteTarget, openDeleteModal, confirmDelete } = useAdminDeleteModal<BgTemplate>({
  deleteById: (id) => bgTemplateApi.adminDelete(id),
  afterDelete: loadList,
  message,
})

onMounted(() => { loadList() })
</script>

<style scoped>
@import './admin-common.css';

.cover-upload-area { display: flex; align-items: center; gap: 16px; }
.cover-preview { width: 80px; height: 106px; border-radius: 12px; overflow: hidden; background: rgba(113, 97, 239, 0.1); border: 1px solid rgba(136, 151, 208, 0.34); flex-shrink: 0; }
.cover-preview.landscape { width: 120px; height: 80px; }
.cover-preview img { width: 100%; height: 100%; object-fit: cover; }
.cover-placeholder { width: 100%; height: 100%; display: flex; align-items: center; justify-content: center; font-size: 12px; color: #8b95b8; }

@media (max-width: 720px) {
  .bg-library-page :deep(.n-select),
  .bg-library-page :deep(.n-button) {
    width: 100%;
  }
}
</style>
