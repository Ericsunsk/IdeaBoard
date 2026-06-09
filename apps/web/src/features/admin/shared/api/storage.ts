import {
  applyAdminStorageProfile,
  createAdminStorageProfile,
  deleteAdminStorageProfile,
  getAdminStorageBucketInfo,
  listAdminStorageProfiles,
  listAdminStorageProviders,
  testAdminStorageProfile,
  updateAdminStorageProfile,
} from '../../../../shared/api/generated-query/admin-storage/admin-storage'
import type { ApiResponse } from '../../../../shared/types'
import type {
  StorageProfile as GeneratedStorageProfile,
  StorageProfilePayload as GeneratedStorageProfilePayload,
  StorageProviderField as GeneratedStorageProviderField,
  StorageProviderMeta as GeneratedStorageProviderMeta,
  CreateAdminStorageProfileParams,
  UpdateAdminStorageProfileParams,
} from '../../../../shared/api/generated-query/model'

export type StorageProvider = string
export type StorageFieldType = string
export type StorageProviderField = Omit<GeneratedStorageProviderField, 'type'> & {
  type: StorageFieldType
}
export type StorageProviderMeta = Omit<GeneratedStorageProviderMeta, 'provider' | 'fields'> & {
  provider: StorageProvider
  fields: StorageProviderField[]
}
export type StorageProfile = Omit<GeneratedStorageProfile, 'provider' | 'secret_configured'> & {
  provider: StorageProvider
  secret_configured: boolean
}
export type StorageProfilePayload = Omit<GeneratedStorageProfilePayload, 'provider'> & {
  provider: StorageProvider
}

function normalizeStorageProfile(profile: GeneratedStorageProfile): StorageProfile {
  return {
    ...profile,
    provider: profile.provider,
    secret_configured: profile.secret_configured ?? false,
  }
}

export const storageApi = {
  getProviders: (): Promise<ApiResponse<{ providers: StorageProviderMeta[] }>> =>
    listAdminStorageProviders(),
  getProfiles: async (): Promise<ApiResponse<{ profiles: StorageProfile[] }>> => {
    const res = await listAdminStorageProfiles()
    return {
      ...res,
      data: res.data && {
        ...res.data,
        profiles: res.data.profiles.map(normalizeStorageProfile),
      },
    }
  },
  createProfile: async (
    data: StorageProfilePayload,
    options?: CreateAdminStorageProfileParams,
  ): Promise<ApiResponse<{ profile: StorageProfile; message: string }>> => {
    const res = await createAdminStorageProfile(data, options)
    return {
      ...res,
      data: res.data && {
        ...res.data,
        profile: normalizeStorageProfile(res.data.profile),
      },
    }
  },
  updateProfile: async (
    id: string,
    data: StorageProfilePayload,
    options?: UpdateAdminStorageProfileParams,
  ): Promise<ApiResponse<{ profile: StorageProfile; message: string }>> => {
    const res = await updateAdminStorageProfile(id, data, options)
    return {
      ...res,
      data: res.data && {
        ...res.data,
        profile: normalizeStorageProfile(res.data.profile),
      },
    }
  },
  deleteProfile: (id: string): Promise<ApiResponse<{ message: string }>> =>
    deleteAdminStorageProfile(id),
  applyProfile: async (
    id: string,
  ): Promise<ApiResponse<{ profile: StorageProfile; message: string }>> => {
    const res = await applyAdminStorageProfile(id)
    return {
      ...res,
      data: res.data && {
        ...res.data,
        profile: normalizeStorageProfile(res.data.profile),
      },
    }
  },
  testProfile: (id: string): Promise<ApiResponse<{ message: string }>> =>
    testAdminStorageProfile(id),
  getBucketInfo: (id: string): Promise<ApiResponse<{ info: { domains?: string[] } }>> =>
    getAdminStorageBucketInfo(id),
}
