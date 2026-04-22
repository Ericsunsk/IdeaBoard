import { ref, type Ref } from 'vue'
import type { UploadFileInfo } from 'naive-ui'
import { UPLOAD_FAILED_MESSAGE, UPLOAD_SUCCESS_MESSAGE } from '../../../shared/constants/media'
import { parseAdminUploadResponse } from './adminUpload'

export const ADMIN_STATUS_OPTIONS = [
  { label: '已上架', value: 1 },
  { label: '已下架', value: 0 },
]

export const ADMIN_ASSET_LIST_QUERY = { page: 1, page_size: 200 } as const
export const ADMIN_STYLE_CARD_LIST_QUERY = { page: 1, page_size: 100 } as const
export const ADMIN_ASSET_PAGE_SIZE = 50
export const ADMIN_STYLE_CARD_PAGE_SIZE = 20

type MessageFn = (text: string) => void
type Message = { success: MessageFn; error: MessageFn }
type ListResponse<T> = { code?: number; data?: { list?: T[] } | null }

export function extractApiErrorMessage(error: unknown, fallback: string): string {
  if (
    typeof error === 'object' &&
    error &&
    'msg' in error &&
    typeof (error as { msg?: unknown }).msg === 'string'
  ) {
    return (error as { msg: string }).msg
  }
  return fallback
}

export function createAdminUploadFinishHandler<T extends object>(
  uploading: Ref<boolean>,
  formData: Ref<T>,
  imageField: keyof T,
  message: Message,
) {
  return ({ file, event }: { file: UploadFileInfo; event?: ProgressEvent }) => {
    uploading.value = false
    const res = parseAdminUploadResponse(event)
    if (res.code === 0 && res.data?.url) {
      const mutableFormData = formData.value as Record<keyof T, unknown>
      mutableFormData[imageField] = res.data.url
      message.success(UPLOAD_SUCCESS_MESSAGE)
    } else {
      message.error(res.msg || UPLOAD_FAILED_MESSAGE)
    }
    return file
  }
}

export function getApiList<T>(response: ListResponse<T> | null | undefined): T[] {
  return response?.data?.list || []
}

export function splitCommaValues(input: string | null | undefined): string[] {
  if (!input) return []
  return input.split(',').map(item => item.trim()).filter(Boolean)
}

export function getChannelModelOptions(
  channels: Array<{ id: number; models?: string | null }>,
  channelId: number | null | undefined,
) {
  if (!channelId) return []
  const channel = channels.find(item => item.id === channelId)
  return splitCommaValues(channel?.models).map(model => ({ label: model, value: model }))
}

export function useAdminListState<T>(options: {
  fetcher: () => Promise<ListResponse<T>>
  message: Message
  errorMessage?: string
}) {
  const list = ref<T[]>([])
  const loading = ref(false)

  async function loadList() {
    loading.value = true
    try {
      const response = await options.fetcher()
      if (response.code === 0) {
        list.value = getApiList(response)
      }
    } catch {
      options.message.error(options.errorMessage || '加载失败')
    } finally {
      loading.value = false
    }
  }

  return { list, loading, loadList }
}

export function useAdminCrudModal<TItem extends { id: number }, TForm extends object>(options: {
  createDefault: () => TForm
  toForm: (item: TItem) => TForm
  create: (payload: TForm) => Promise<unknown>
  update: (id: number, payload: TForm) => Promise<unknown>
  afterSave: () => Promise<unknown>
  message: Message
  successText?: { create?: string; update?: string }
  fallbackErrorMessage?: string
  transformPayload?: (payload: TForm) => TForm
}) {
  const showModal = ref(false)
  const isEditing = ref(false)
  const saving = ref(false)
  const formRef = ref<{ validate?: () => Promise<void> } | null>(null)
  const formData = ref<TForm>(options.createDefault())
  const editingId = ref<number | null>(null)

  function openAddModal() {
    isEditing.value = false
    editingId.value = null
    formData.value = options.createDefault()
    showModal.value = true
  }

  function openEditModal(item: TItem) {
    isEditing.value = true
    editingId.value = item.id
    formData.value = options.toForm(item)
    showModal.value = true
  }

  async function handleSave() {
    try {
      await formRef.value?.validate?.()
    } catch {
      return
    }

    saving.value = true
    try {
      const payload = options.transformPayload ? options.transformPayload(formData.value) : formData.value
      if (isEditing.value && editingId.value) {
        await options.update(editingId.value, payload)
        options.message.success(options.successText?.update || '更新成功')
      } else {
        await options.create(payload)
        options.message.success(options.successText?.create || '创建成功')
      }
      showModal.value = false
      await options.afterSave()
    } catch (error) {
      options.message.error(extractApiErrorMessage(error, options.fallbackErrorMessage || '操作失败'))
    } finally {
      saving.value = false
    }
  }

  return {
    showModal,
    isEditing,
    saving,
    formRef,
    formData,
    editingId,
    openAddModal,
    openEditModal,
    handleSave,
  }
}

export function useAdminDeleteModal<TItem extends { id: number }>(options: {
  deleteById: (id: number) => Promise<unknown>
  afterDelete: () => Promise<unknown>
  message: Message
  successText?: string
  fallbackErrorMessage?: string
}) {
  const showDeleteModal = ref(false)
  const deleting = ref(false)
  const deleteTarget = ref<TItem | null>(null)

  function openDeleteModal(item: TItem) {
    deleteTarget.value = item
    showDeleteModal.value = true
  }

  async function confirmDelete() {
    if (!deleteTarget.value) return
    deleting.value = true
    try {
      await options.deleteById(deleteTarget.value.id)
      options.message.success(options.successText || '删除成功')
      showDeleteModal.value = false
      await options.afterDelete()
    } catch (error) {
      options.message.error(extractApiErrorMessage(error, options.fallbackErrorMessage || '删除失败'))
    } finally {
      deleting.value = false
    }
  }

  return {
    showDeleteModal,
    deleting,
    deleteTarget,
    openDeleteModal,
    confirmDelete,
  }
}
