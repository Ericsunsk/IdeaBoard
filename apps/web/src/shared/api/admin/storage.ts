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

type StorageRequestClient = {
  get: <T>(url: string, config?: Record<string, unknown>) => Promise<ApiResponse<T>>
  post: <T, D = unknown>(url: string, data?: D, config?: Record<string, unknown>) => Promise<ApiResponse<T>>
  put: <T, D = unknown>(url: string, data?: D, config?: Record<string, unknown>) => Promise<ApiResponse<T>>
  delete: <T>(url: string, config?: Record<string, unknown>) => Promise<ApiResponse<T>>
}

export function createStorageApi(client: StorageRequestClient) {
  return {
    getProviders: (): Promise<ApiResponse<{ providers: StorageProviderMeta[] }>> =>
      client.get<{ providers: StorageProviderMeta[] }>('/admin/storage/providers'),
    getProfiles: (): Promise<ApiResponse<{ profiles: StorageProfile[] }>> =>
      client.get<{ profiles: StorageProfile[] }>('/admin/storage/profiles'),
    createProfile: (data: StorageProfilePayload): Promise<ApiResponse<{ profile: StorageProfile; message: string }>> =>
      client.post<{ profile: StorageProfile; message: string }, StorageProfilePayload>('/admin/storage/profiles', data),
    updateProfile: (id: string, data: StorageProfilePayload): Promise<ApiResponse<{ profile: StorageProfile; message: string }>> =>
      client.put<{ profile: StorageProfile; message: string }, StorageProfilePayload>('/admin/storage/profiles/' + id, data),
    deleteProfile: (id: string): Promise<ApiResponse<{ message: string }>> =>
      client.delete<{ message: string }>('/admin/storage/profiles/' + id),
    applyProfile: (id: string): Promise<ApiResponse<{ profile: StorageProfile; message: string }>> =>
      client.post<{ profile: StorageProfile; message: string }>('/admin/storage/profiles/' + id + '/apply'),
    testProfile: (id: string): Promise<ApiResponse<{ message: string }>> =>
      client.post<{ message: string }>('/admin/storage/profiles/' + id + '/test'),
    getBucketInfo: (id: string): Promise<ApiResponse<{ info: { domains?: string[] } }>> =>
      client.get<{ info: { domains?: string[] } }>('/admin/storage/profiles/' + id + '/bucket-info'),
  }
}
