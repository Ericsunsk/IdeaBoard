<template>
  <div class="admin-page style-cards-page">
    <div class="page-header">
      <div>
        <h2 class="page-title">风格卡片管理</h2>
        <p class="page-desc">维护用户可见风格素材，支持上下架、互斥与排序控制。</p>
      </div>
      <div class="header-side-note">
        <span>总数 {{ cards.length }}</span>
      </div>
    </div>

    <div class="summary-grid">
      <AdminSummaryCard label="总风格" :value="cards.length" foot="包含下架" />
      <AdminSummaryCard label="已上架" :value="enabledCardCount" foot="用户可见" />
      <AdminSummaryCard label="互斥模式" :value="exclusiveCardCount" foot="单选风格" />
      <AdminSummaryCard label="筛选结果" :value="filteredCards.length" foot="当前列表" />
    </div>

    <div class="toolbar">
      <n-input
        v-model:value="searchKeyword"
        placeholder="搜索风格名称..."
        clearable
        style="width: 200px"
      >
        <template #prefix>
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/></svg>
        </template>
      </n-input>
      <n-select
        v-model:value="filterEnabled"
        :options="statusOptions"
        placeholder="上下架状态"
        clearable
        style="width: 140px"
      />
      <n-button @click="loadCards">
        <template #icon>
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="23 4 23 10 17 10"/><polyline points="1 20 1 14 7 14"/><path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15"/></svg>
        </template>
        刷新
      </n-button>
      <n-button type="primary" @click="openAddModal">
        <template #icon>
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>
        </template>
        新增风格
      </n-button>
    </div>

    <!-- 卡片列表 -->
    <div class="table-card">
      <n-data-table
        :columns="columns"
        :data="filteredCards"
        :loading="loading"
        :pagination="false"
        :bordered="false"
        :row-key="(row: StyleCard) => row.id"
      />
      <div class="pagination-wrapper">
        <n-pagination
          v-model:page="page"
          :page-count="pageCount"
          @update:page="loadCards"
        />
      </div>
    </div>

    <!-- 新增/编辑弹窗 -->
    <n-modal
      v-model:show="showModal"
      preset="card"
      :title="isEditing ? '编辑风格' : '新增风格'"
      :style="{ maxWidth: '560px' }"
    >
      <n-form
        ref="formRef"
        :model="formData"
        :rules="rules"
        label-placement="top"
        size="large"
      >
        <n-form-item label="风格名称" path="name">
          <n-input v-model:value="formData.name" placeholder="如：水彩插画、赛博朋克" />
        </n-form-item>

        <n-form-item label="封面图片" path="cover_image">
          <AdminImageUploadField
            :image-url="formData.cover_image"
            empty-text="暂无封面"
            alt="封面预览"
            :accept="IMAGE_UPLOAD_ACCEPT_EXTENDED"
            :loading="uploading"
            :action="adminUploadAction"
            :headers="uploadHeaders"
            upload-text="上传封面"
            @finish="handleUploadFinish"
            @start-upload="uploading = true"
          />
        </n-form-item>

        <n-form-item label="内部关键词（用户不可见）" path="keywords">
          <n-input
            v-model:value="formData.keywords"
            type="textarea"
            placeholder="多个关键词用逗号分隔，如：watercolor, soft colors, delicate"
            :rows="3"
          />
          <template #feedback>
            <span style="color: #8b95b8; font-size: 12px;">用户选择此风格时，系统自动将此关键词拼接到描述前，全程用户不可见</span>
          </template>
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

        <n-form-item label="是否互斥（单选模式）">
          <n-switch v-model:value="formData.is_exclusive">
            <template #checked>是</template>
            <template #unchecked>否</template>
          </n-switch>
          <template #feedback>
            <span style="color: #8b95b8; font-size: 12px;">互斥模式下，用户只能选择该风格卡片中的一个；非互斥则可多选</span>
          </template>
        </n-form-item>
      </n-form>

      <template #footer>
        <div class="modal-footer">
          <n-button @click="showModal = false">取消</n-button>
          <n-button type="primary" :loading="saving" @click="handleSave">保存</n-button>
        </div>
      </template>
    </n-modal>

    <!-- 删除确认弹窗 -->
    <n-modal
      v-model:show="showDeleteModal"
      preset="card"
      title="确认删除"
      :style="{ maxWidth: '400px' }"
    >
      <p style="color: #7884ad; line-height: 1.8;">确定要删除风格「{{ deleteTarget?.name }}」吗？此操作不可撤销。</p>
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
  NDataTable, NPagination, NButton, NInput, NModal, NForm, NFormItem,
  NSwitch, NGrid, NGi, NInputNumber, NSelect, useMessage
} from 'naive-ui'
import { styleCardApi } from '../../../shared/api/index'
import type { StyleCard, StyleCardFormData } from '../../../shared/api/index'
import { IMAGE_UPLOAD_ACCEPT_EXTENDED } from '../../../shared/constants/media'
import { adminUploadAction, useAdminUploadHeaders } from '../shared/adminUpload'
import {
  ADMIN_STATUS_OPTIONS,
  ADMIN_STYLE_CARD_LIST_QUERY,
  ADMIN_STYLE_CARD_PAGE_SIZE,
  createAdminUploadFinishHandler,
  useAdminCrudModal,
  useAdminDeleteModal,
  useAdminListState,
} from '../shared/assetLibraryShared'
import AdminImageUploadField from '../shared/components/AdminImageUploadField.vue'
import AdminSummaryCard from '../shared/components/AdminSummaryCard.vue'

const message = useMessage()

// ============ 列表数据 ============
const { list: cards, loading, loadList: loadCards } = useAdminListState<StyleCard>({
  fetcher: () => styleCardApi.getList(ADMIN_STYLE_CARD_LIST_QUERY),
  message,
  errorMessage: '加载失败',
})
const page = ref(1)
const pageSize = ADMIN_STYLE_CARD_PAGE_SIZE
const pageCount = computed(() => Math.max(1, Math.ceil(cards.value.length / pageSize)))

const searchKeyword = ref('')
const filterEnabled = ref<number | null>(null)

const statusOptions = ADMIN_STATUS_OPTIONS

const filteredCards = computed(() => {
  let result = cards.value
  if (searchKeyword.value) {
    result = result.filter(c => c.name.includes(searchKeyword.value))
  }
  if (filterEnabled.value !== null) {
    result = result.filter(c => c.is_enabled === (filterEnabled.value === 1))
  }
  return result
})
const enabledCardCount = computed(() => cards.value.filter(item => item.is_enabled).length)
const exclusiveCardCount = computed(() => cards.value.filter(item => item.is_exclusive).length)

// ============ 表格列定义 ============
const columns = [
  {
    title: '封面',
    key: 'cover_image',
    width: 80,
    render(row: StyleCard) {
      return h('div', {
        style: 'width: 60px; height: 40px; border-radius: 10px; overflow: hidden; background: rgba(113, 97, 239, 0.1);'
      }, [
        h('img', {
          src: row.cover_image,
          style: 'width: 100%; height: 100%; object-fit: cover;',
          onError: (e: Event) => {
            const img = e.target as HTMLImageElement
            img.style.display = 'none'
          }
        })
      ])
    }
  },
  { title: '名称', key: 'name', ellipsis: { tooltip: true } },
  {
    title: '关键词',
    key: 'keywords',
    ellipsis: { tooltip: true },
    render() {
      return h('span', { style: 'font-size: 12px; color: #8b95b8;' }, '（隐藏）')
    }
  },
  {
    title: '排序',
    key: 'sort_order',
    width: 70,
    render(row: StyleCard) {
      return h('span', { style: 'color: #8b95b8;' }, row.sort_order)
    }
  },
  {
    title: '互斥',
    key: 'is_exclusive',
    width: 70,
    render(row: StyleCard) {
      return h('span', {
        style: row.is_exclusive ? 'color: #6460db;' : 'color: #8b95b8;'
      }, row.is_exclusive ? '是' : '否')
    }
  },
  {
    title: '状态',
    key: 'is_enabled',
    width: 80,
    render(row: StyleCard) {
      return h('span', {
        style: row.is_enabled ? 'color: #1f9d68;' : 'color: #d65478;'
      }, row.is_enabled ? '已上架' : '已下架')
    }
  },
  {
    title: '操作',
    key: 'actions',
    width: 140,
    render(row: StyleCard) {
      return h('div', { style: 'display: flex; gap: 8px;' }, [
        h(NButton, {
          size: 'small',
          secondary: true,
          onClick: () => openEditModal(row)
        }, { default: () => '编辑' }),
        h(NButton, {
          size: 'small',
          tertiary: true,
          style: 'color: #d65478;',
          onClick: () => openDeleteModal(row)
        }, { default: () => '删除' })
      ])
    }
  }
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
} = useAdminCrudModal<StyleCard, StyleCardFormData>({
  createDefault: () => ({
    name: '',
    cover_image: '',
    keywords: '',
    sort_order: 0,
    is_enabled: true,
    is_exclusive: false,
  }),
  toForm: (item) => ({
    name: item.name,
    cover_image: item.cover_image,
    keywords: '',
    sort_order: item.sort_order,
    is_enabled: item.is_enabled,
    is_exclusive: item.is_exclusive,
  }),
  create: (payload) => styleCardApi.create(payload),
  update: (id, payload) => styleCardApi.update(id, payload),
  afterSave: loadCards,
  message,
  transformPayload: (payload) => ({ ...payload }),
})
const uploading = ref(false)

const uploadHeaders = useAdminUploadHeaders()
const handleUploadFinish = createAdminUploadFinishHandler(uploading, formData, 'cover_image', message)

const rules = {
  name: [{ required: true, message: '请输入风格名称', trigger: 'blur' }],
  keywords: [{ required: true, message: '请输入内部关键词', trigger: 'blur' }],
}

// ============ 删除 ============
const { showDeleteModal, deleting, deleteTarget, openDeleteModal, confirmDelete } = useAdminDeleteModal<StyleCard>({
  deleteById: (id) => styleCardApi.delete(id),
  afterDelete: loadCards,
  message,
})

onMounted(() => {
  loadCards()
})
</script>

<style scoped>
@import '../shared/adminCommon.css';

.style-cards-page {
  gap: 14px;
}

.modal-footer {
  gap: 8px;
}
</style>
