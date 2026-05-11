import { getAdminStorage } from '../generated/admin-storage/admin-storage'
import type { ApiResponse } from '../types'
import type {
  StorageProfile as GeneratedStorageProfile,
  StorageProfilePayload as GeneratedStorageProfilePayload,
  StorageProviderField as GeneratedStorageProviderField,
  StorageProviderMeta as GeneratedStorageProviderMeta,
} from '../generated/model'

const adminStorageApi = getAdminStorage()

export type StorageProvider = string
export type StorageFieldType = string
export type StorageProviderField = Omit<GeneratedStorageProviderField, 'type'> & { type: StorageFieldType }
export type StorageProviderMeta = Omit<GeneratedStorageProviderMeta, 'provider' | 'fields'> & {
  provider: StorageProvider
  fields: StorageProviderField[]
}
export type StorageProfile = Omit<GeneratedStorageProfile, 'provider'> & { provider: StorageProvider }
export type StorageProfilePayload = Omit<GeneratedStorageProfilePayload, 'provider'> & { provider: StorageProvider }

export const storageApi = {
  getProviders: (): Promise<ApiResponse<{ providers: StorageProviderMeta[] }>> => adminStorageApi.listAdminStorageProviders(),
  getProfiles: (): Promise<ApiResponse<{ profiles: StorageProfile[] }>> => adminStorageApi.listAdminStorageProfiles(),
  createProfile: (data: StorageProfilePayload): Promise<ApiResponse<{ profile: StorageProfile; message: string }>> =>
    adminStorageApi.createAdminStorageProfile(data),
  updateProfile: (id: string, data: StorageProfilePayload): Promise<ApiResponse<{ profile: StorageProfile; message: string }>> =>
    adminStorageApi.updateAdminStorageProfile(id, data),
  deleteProfile: (id: string): Promise<ApiResponse<{ message: string }>> => adminStorageApi.deleteAdminStorageProfile(id),
  applyProfile: (id: string): Promise<ApiResponse<{ profile: StorageProfile; message: string }>> =>
    adminStorageApi.applyAdminStorageProfile(id),
  testProfile: (id: string): Promise<ApiResponse<{ message: string }>> => adminStorageApi.testAdminStorageProfile(id),
  getBucketInfo: (id: string): Promise<ApiResponse<{ info: { domains?: string[] } }>> => adminStorageApi.getAdminStorageBucketInfo(id),
}
