import {
  listAdminSearchProviders,
  testAdminSearchProvider,
  updateAdminSearchProvider,
} from '../../../shared/api/generated-query/admin-search-providers/admin-search-providers'
import type {
  SearchProvider,
  SearchProviderProvider,
  SearchProviderTestResult,
} from '../../../shared/api/generated-query/model'
import { resolveApiData, resolveApiOk } from '../../../shared/api/apiResponse'

export type SearchProviderRecord = SearchProvider
export type SearchProviderKey = SearchProviderProvider

export async function fetchSearchProviders(): Promise<SearchProviderRecord[]> {
  const data = await resolveApiData(listAdminSearchProviders(), '加载搜索供应商配置失败')
  return data.providers ?? []
}

export async function saveSearchProvider(
  provider: SearchProviderKey,
  input: { apiKey?: string; enabled: boolean },
): Promise<void> {
  await resolveApiOk(
    updateAdminSearchProvider(provider, {
      ...(input.apiKey?.trim() ? { api_key: input.apiKey.trim() } : {}),
      enabled: input.enabled,
    }),
    '保存搜索供应商配置失败',
  )
}

export async function testSearchProvider(
  provider: SearchProviderKey,
): Promise<SearchProviderTestResult> {
  const data = await resolveApiData(testAdminSearchProvider(provider), '搜索供应商连接测试失败')
  return data
}
