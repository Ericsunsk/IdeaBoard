import {
  listAdminSearchProviders,
  testAdminSearchProvider,
  updateAdminSearchProvider,
} from '../../../shared/api/generated-query/admin-search-providers/admin-search-providers'
import { resolveApiData, resolveApiOk } from '../../../shared/api/apiResponse'
import type {
  SearchProviderId,
  SearchProviderList,
  SearchProviderTestResult,
  SearchProviderUpdate,
} from './searchProviderTypes'

export interface AdminSearchProviderResource {
  list: () => Promise<SearchProviderList>
  update: (provider: SearchProviderId, input: SearchProviderUpdate) => Promise<void>
  test: (provider: SearchProviderId) => Promise<SearchProviderTestResult>
}

export function createAdminSearchProviderResource(): AdminSearchProviderResource {
  return {
    list: listSearchProviders,
    update: updateSearchProvider,
    test: testSearchProvider,
  }
}

function listSearchProviders() {
  return resolveApiData<SearchProviderList>(
    listAdminSearchProviders(),
    '加载图片搜索供应商配置失败',
  )
}

function updateSearchProvider(provider: SearchProviderId, input: SearchProviderUpdate) {
  return resolveApiOk(updateAdminSearchProvider(provider, input), '保存图片搜索供应商配置失败')
}

function testSearchProvider(provider: SearchProviderId) {
  return resolveApiData<SearchProviderTestResult>(
    testAdminSearchProvider(provider),
    '测试图片搜索供应商连接失败',
  )
}
