import {
  applyAdminStorageProfile,
  createAdminStorageProfile,
  deleteAdminStorageProfile,
  getAdminStorageBucketInfo,
  listAdminStorageProfiles,
  listAdminStorageProviders,
  testAdminStorageProfile,
  updateAdminStorageProfile,
} from '../generated-query/admin-storage/admin-storage'
import type { ApiResponse } from '../../types'
import type {
  StorageProfile as GeneratedStorageProfile,
  StorageProfilePayload as GeneratedStorageProfilePayload,
  StorageProviderField as GeneratedStorageProviderField,
  StorageProviderMeta as GeneratedStorageProviderMeta,
} from '../generated-query/model'

export type StorageProvider = string
export type StorageFieldType = string
export type StorageProviderField = Omit<GeneratedStorageProviderField, 'type'> & {
  type: StorageFieldType
}
export type StorageProviderMeta = Omit<GeneratedStorageProviderMeta, 'provider' | 'fields'> & {
  provider: StorageProvider
  fields: StorageProviderField[]
}
export type StorageProfile = Omit<GeneratedStorageProfile, 'provider'> & {
  provider: StorageProvider
}
export type StorageProfilePayload = Omit<GeneratedStorageProfilePayload, 'provider'> & {
  provider: StorageProvider
}

export const storageApi = {
  getProviders: (): Promise<ApiResponse<{ providers: StorageProviderMeta[] }>> =>
    listAdminStorageProviders(),
  getProfiles: (): Promise<ApiResponse<{ profiles: StorageProfile[] }>> =>
    listAdminStorageProfiles(),
  createProfile: (
    data: StorageProfilePayload,
  ): Promise<ApiResponse<{ profile: StorageProfile; message: string }>> =>
    createAdminStorageProfile(data),
  updateProfile: (
    id: string,
    data: StorageProfilePayload,
  ): Promise<ApiResponse<{ profile: StorageProfile; message: string }>> =>
    updateAdminStorageProfile(id, data),
  deleteProfile: (id: string): Promise<ApiResponse<{ message: string }>> =>
    deleteAdminStorageProfile(id),
  applyProfile: (id: string): Promise<ApiResponse<{ profile: StorageProfile; message: string }>> =>
    applyAdminStorageProfile(id),
  testProfile: (id: string): Promise<ApiResponse<{ message: string }>> =>
    testAdminStorageProfile(id),
  getBucketInfo: (id: string): Promise<ApiResponse<{ info: { domains?: string[] } }>> =>
    getAdminStorageBucketInfo(id),
}
