import type {
  CreateAdminStorageProfileParams,
  UpdateAdminStorageProfileParams,
} from '../../../../shared/api/generated-query/model'
import { storageApi, type StorageProfile, type StorageProfilePayload } from '../api/storage'
import { requireApiData } from './adminApiResponse'

type SaveStorageProfileInput =
  | {
      id?: undefined
      payload: StorageProfilePayload
      options?: CreateAdminStorageProfileParams
    }
  | {
      id: string
      payload: StorageProfilePayload
      options?: UpdateAdminStorageProfileParams
    }

export async function saveStorageProfile(
  input: SaveStorageProfileInput,
): Promise<{ profile: StorageProfile; message: string }> {
  const res = input.id
    ? await storageApi.updateProfile(input.id, input.payload, input.options)
    : await storageApi.createProfile(input.payload, input.options)
  return requireApiData(res, '保存存储配置失败')
}

export async function applyStorageProfile(
  id: string,
): Promise<{ profile: StorageProfile; message: string }> {
  const res = await storageApi.applyProfile(id)
  return requireApiData(res, '设为当前失败')
}

export async function testStorageProfile(id: string): Promise<{ message: string }> {
  const res = await storageApi.testProfile(id)
  return requireApiData(res, '连接失败')
}

export async function deleteStorageProfile(id: string): Promise<{ message: string }> {
  const res = await storageApi.deleteProfile(id)
  return requireApiData(res, '删除失败')
}

export async function fetchStorageBucketDomains(profileId: string): Promise<string[]> {
  const res = await storageApi.getBucketInfo(profileId)
  const data = requireApiData(res, '获取 Bucket 信息失败')
  return data.info.domains ?? []
}
