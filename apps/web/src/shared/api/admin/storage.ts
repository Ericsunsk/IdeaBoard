import type { AxiosInstance } from 'axios'
import type { ApiResponse } from '../types'

export type StorageProvider = 'local' | 'kodo' | 's3-compatible' | string
export type StorageFieldType = 'text' | 'password' | 'switch' | string

export interface StorageProviderField {
  key: string
  label: string
  type: StorageFieldType
  placeholder?: string
  hint?: string
  required?: boolean
  secret?: boolean
}

export interface StorageProviderMeta {
  provider: StorageProvider
  label: string
  description: string
  builtin: boolean
  create_allowed: boolean
  supports_test: boolean
  supports_bucket_info: boolean
  fields: StorageProviderField[]
}

export interface StorageProfile {
  id: string
  name: string
  provider: StorageProvider
  enabled: boolean
  is_current: boolean
  builtin: boolean
  secret_configured: boolean
  config: Record<string, unknown>
}

export interface StorageProfilePayload {
  name: string
  provider: StorageProvider
  enabled: boolean
  config: Record<string, unknown>
}

export function createStorageApi(api: AxiosInstance) {
  return {
    getProviders: (): Promise<ApiResponse<{ providers: StorageProviderMeta[] }>> =>
      api.get('/admin/storage/providers') as unknown as Promise<ApiResponse<{ providers: StorageProviderMeta[] }>>,
    getProfiles: (): Promise<ApiResponse<{ profiles: StorageProfile[] }>> =>
      api.get('/admin/storage/profiles') as unknown as Promise<ApiResponse<{ profiles: StorageProfile[] }>>,
    createProfile: (data: StorageProfilePayload): Promise<ApiResponse<{ profile: StorageProfile; message: string }>> =>
      api.post('/admin/storage/profiles', data) as unknown as Promise<ApiResponse<{ profile: StorageProfile; message: string }>>,
    updateProfile: (id: string, data: StorageProfilePayload): Promise<ApiResponse<{ profile: StorageProfile; message: string }>> =>
      api.put('/admin/storage/profiles/' + id, data) as unknown as Promise<ApiResponse<{ profile: StorageProfile; message: string }>>,
    deleteProfile: (id: string): Promise<ApiResponse<{ message: string }>> =>
      api.delete('/admin/storage/profiles/' + id) as unknown as Promise<ApiResponse<{ message: string }>>,
    applyProfile: (id: string): Promise<ApiResponse<{ profile: StorageProfile; message: string }>> =>
      api.post('/admin/storage/profiles/' + id + '/apply') as unknown as Promise<ApiResponse<{ profile: StorageProfile; message: string }>>,
    testProfile: (id: string): Promise<ApiResponse<{ message: string }>> =>
      api.post('/admin/storage/profiles/' + id + '/test') as unknown as Promise<ApiResponse<{ message: string }>>,
    getBucketInfo: (id: string): Promise<ApiResponse<{ info: { domains?: string[] } }>> =>
      api.get('/admin/storage/profiles/' + id + '/bucket-info') as unknown as Promise<ApiResponse<{ info: { domains?: string[] } }>>,
  }
}
