import { apiDelete, apiGet, apiPost, apiPut } from './client'
import { createStorageApi } from './admin/storage'

export const storageApi = createStorageApi({
  get: apiGet,
  post: apiPost,
  put: apiPut,
  delete: apiDelete,
})

export type {
  StorageFieldType,
  StorageProfile,
  StorageProfilePayload,
  StorageProvider,
  StorageProviderField,
  StorageProviderMeta,
} from './admin/storage'
