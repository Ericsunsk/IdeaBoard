<template>
  <div class="style-cards-page">
    <!-- 工具栏 -->
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
        :row-key="(row: any) => row.id"
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
                <template #icon>
                  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="17 8 12 3 7 8"/><line x1="12" y1="3" x2="12" y2="15"/></svg>
                </template>
                {{ formData.cover_image ? '重新上传' : '上传封面' }}
              </n-button>
            </n-upload>
          </div>
        </n-form-item>

        <n-form-item label="内部关键词（用户不可见）" path="keywords">
          <n-input
            v-model:value="formData.keywords"
            type="textarea"
            placeholder="多个关键词用逗号分隔，如：watercolor, soft colors, delicate"
            :rows="3"
          />
          <template #feedback>
            <span style="color: #71717a; font-size: 12px;">用户选择此风格时，系统自动将此关键词拼接到描述前，全程用户不可见</span>
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
            <span style="color: #71717a; font-size: 12px;">互斥模式下，用户只能选择该风格卡片中的一个；非互斥则可多选</span>
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
      <p style="color: #a1a1aa; line-height: 1.8;">确定要删除风格「{{ deleteTarget?.name }}」吗？此操作不可撤销。</p>
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
  NSwitch, NGrid, NGi, NInputNumber, NUpload, NSelect, useMessage
} from 'naive-ui'
import type { UploadFileInfo } from 'naive-ui'
import { styleCardApi } from '../../api/index'
import type { StyleCard } from '../../api/index'
import { IMAGE_UPLOAD_ACCEPT_EXTENDED, UPLOAD_FAILED_MESSAGE, UPLOAD_SUCCESS_MESSAGE } from '../../constants/media'
import { adminUploadAction, parseAdminUploadResponse, useAdminUploadHeaders } from '../../utils/admin-upload'

const message = useMessage()

// ============ 列表数据 ============
const cards = ref<StyleCard[]>([])
const loading = ref(false)
const page = ref(1)
const pageSize = 20
const pageCount = computed(() => Math.max(1, Math.ceil(cards.value.length / pageSize)))

const searchKeyword = ref('')
const filterEnabled = ref<number | null>(null)

const statusOptions = [
  { label: '已上架', value: 1 },
  { label: '已下架', value: 0 },
]

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

// ============ 表格列定义 ============
const columns = [
  {
    title: '封面',
    key: 'cover_image',
    width: 80,
    render(row: StyleCard) {
      return h('div', {
        style: 'width: 60px; height: 40px; border-radius: 6px; overflow: hidden; background: #27272a;'
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
      return h('span', { style: 'font-size: 12px; color: #71717a;' }, '（隐藏）')
    }
  },
  {
    title: '排序',
    key: 'sort_order',
    width: 70,
    render(row: StyleCard) {
      return h('span', { style: 'color: #71717a;' }, row.sort_order)
    }
  },
  {
    title: '互斥',
    key: 'is_exclusive',
    width: 70,
    render(row: StyleCard) {
      return h('span', {
        style: row.is_exclusive ? 'color: #6366f1;' : 'color: #52525b;'
      }, row.is_exclusive ? '是' : '否')
    }
  },
  {
    title: '状态',
    key: 'is_enabled',
    width: 80,
    render(row: StyleCard) {
      return h('span', {
        style: row.is_enabled ? 'color: #22c55e;' : 'color: #ef4444;'
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
          style: 'color: #ef4444;',
          onClick: () => openDeleteModal(row)
        }, { default: () => '删除' })
      ])
    }
  }
]

// ============ 新增/编辑 ============
const showModal = ref(false)
const isEditing = ref(false)
const saving = ref(false)
const formRef = ref()
const formData = ref({
  name: '',
  cover_image: '',
  keywords: '',
  sort_order: 0,
  is_enabled: true,
  is_exclusive: false,
})
const editingId = ref<number | null>(null)
const uploading = ref(false)

const uploadHeaders = useAdminUploadHeaders()

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

const rules = {
  name: [{ required: true, message: '请输入风格名称', trigger: 'blur' }],
  keywords: [{ required: true, message: '请输入内部关键词', trigger: 'blur' }],
}

function openAddModal() {
  isEditing.value = false
  editingId.value = null
  formData.value = {
    name: '',
    cover_image: '',
    keywords: '',
    sort_order: 0,
    is_enabled: true,
    is_exclusive: false,
  }
  showModal.value = true
}

function openEditModal(card: StyleCard) {
  isEditing.value = true
  editingId.value = card.id
  formData.value = {
    name: card.name,
    cover_image: card.cover_image,
    keywords: '',
    sort_order: card.sort_order,
    is_enabled: card.is_enabled,
    is_exclusive: card.is_exclusive,
  }
  showModal.value = true
}

async function handleSave() {
  try {
    await formRef.value?.validate()
  } catch {
    return
  }

  saving.value = true
  try {
    if (isEditing.value && editingId.value) {
      await (styleCardApi as any).update(editingId.value, {
        name: formData.value.name,
        cover_image: formData.value.cover_image,
        keywords: formData.value.keywords,
        sort_order: formData.value.sort_order,
        is_enabled: formData.value.is_enabled,
        is_exclusive: formData.value.is_exclusive,
      })
      message.success('更新成功')
    } else {
      await (styleCardApi as any).create({
        name: formData.value.name,
        cover_image: formData.value.cover_image,
        keywords: formData.value.keywords,
        sort_order: formData.value.sort_order,
        is_enabled: formData.value.is_enabled,
        is_exclusive: formData.value.is_exclusive,
      })
      message.success('创建成功')
    }
    showModal.value = false
    await loadCards()
  } catch (e: any) {
    message.error(e.msg || '操作失败')
  } finally {
    saving.value = false
  }
}

// ============ 删除 ============
const showDeleteModal = ref(false)
const deleting = ref(false)
const deleteTarget = ref<StyleCard | null>(null)

function openDeleteModal(card: StyleCard) {
  deleteTarget.value = card
  showDeleteModal.value = true
}

async function confirmDelete() {
  if (!deleteTarget.value) return
  deleting.value = true
  try {
    await (styleCardApi as any).delete(deleteTarget.value.id)
    message.success('删除成功')
    showDeleteModal.value = false
    await loadCards()
  } catch (e: any) {
    message.error(e.msg || '删除失败')
  } finally {
    deleting.value = false
  }
}

// ============ 加载数据 ============
async function loadCards() {
  loading.value = true
  try {
    const res: any = await (styleCardApi as any).getList({ page: 1, page_size: 100 })
    if (res.code === 0) {
      cards.value = res.data?.list || []
    }
  } catch (e) {
    console.error('loadCards error:', e)
    message.error('加载失败')
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  loadCards()
})
</script>

<style scoped>
.style-cards-page {
  padding: 28px 32px;
  display: flex;
  flex-direction: column;
  gap: 20px;
  max-width: 1400px;
}

.toolbar {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
}

.table-card {
  background: #18181b;
  border-radius: 12px;
  border: 1px solid #27272a;
  padding: 16px;
}

.pagination-wrapper {
  margin-top: 16px;
  display: flex;
  justify-content: flex-end;
}

.cover-upload-area {
  display: flex;
  align-items: center;
  gap: 16px;
}

.cover-preview {
  width: 120px;
  height: 80px;
  border-radius: 8px;
  overflow: hidden;
  background: #27272a;
  border: 1px solid #3f3f46;
  flex-shrink: 0;
}

.cover-preview img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.cover-placeholder {
  width: 100%;
  height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 12px;
  color: #52525b;
}

.modal-footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}

:deep(.n-input) {
  --n-border: 1px solid #27272a;
  --n-border-hover: 1px solid #6366f1;
  --n-border-focus: 1px solid #6366f1;
  --n-color: #18181b;
  --n-color-focus: #18181b;
  --n-text-color: #e4e4e7;
  --n-placeholder-color: #52525b;
  border-radius: 8px;
}

:deep(.n-button--primary-type) {
  --n-color: #6366f1;
  --n-color-hover: #818cf8;
  --n-color-pressed: #4f46e5;
  border-radius: 8px;
}

:deep(.n-button--error-type) {
  --n-color: #ef4444;
  --n-color-hover: #f87171;
  border-radius: 8px;
}

:deep(.n-data-table) {
  --n-td-color: transparent;
  --n-th-color: transparent;
  --n-border-color: #27272a;
  --n-td-color-hover: rgba(99, 102, 241, 0.05);
  --n-th-text-color: #71717a;
  --n-td-text-color: #e4e4e7;
}
</style>
