import { buildControlMutationHeaders, type ControlMutationOptions } from '../../../shared/index'
import {
  listControlSearchProviders,
  testControlSearchProvider,
  updateControlSearchProvider,
} from '@/api/generated-query/control/control-platform/control-platform'
import { requireContractData } from '@/api/http/contractKernel'
import { withControlCopy } from '../../../shared/index'
import type {
  SearchProviderId,
  SearchProviderList,
  SearchProviderTestResult,
  SearchProviderUpdate,
} from './searchProviderTypes'

type SearchProviderMutationOptions = ControlMutationOptions

export interface AdminSearchProviderResource {
  list: () => Promise<SearchProviderList>
  update: (
    provider: SearchProviderId,
    input: SearchProviderUpdate,
    options?: SearchProviderMutationOptions,
  ) => Promise<void>
  test: (
    provider: SearchProviderId,
    options?: SearchProviderMutationOptions,
  ) => Promise<SearchProviderTestResult>
}

export function createAdminSearchProviderResource(): AdminSearchProviderResource {
  return {
    list: listSearchProviders,
    update: updateSearchProvider,
    test: testSearchProvider,
  }
}

function listSearchProviders() {
  // Wire-identical to the retired hand-rolled GET (same `{ data }` body
  // through `controlMutator`); the generated hook owns the route so the
  // duplicate URL prefixing is deleted.
  return (async () => {
    try {
      const body = await listControlSearchProviders()
      return requireContractData<SearchProviderList>(body, '加载图片搜索供应商配置失败')
    } catch (error) {
      throw withControlCopy(error, '加载图片搜索供应商配置失败')
    }
  })()
}

async function updateSearchProvider(
  provider: SearchProviderId,
  input: SearchProviderUpdate,
  options?: SearchProviderMutationOptions,
): Promise<void> {
  try {
    const body = await updateControlSearchProvider(
      provider,
      { ...input },
      { headers: buildControlMutationHeaders(options) },
    )
    // Nil-ack mutation: success is `{ data: {} }`.
    requireContractData<Record<string, unknown>>(body, '保存图片搜索供应商配置失败')
  } catch (error) {
    throw withControlCopy(error, '保存图片搜索供应商配置失败')
  }
}

async function testSearchProvider(
  provider: SearchProviderId,
  options?: SearchProviderMutationOptions,
): Promise<SearchProviderTestResult> {
  try {
    const body = await testControlSearchProvider(provider, {
      headers: buildControlMutationHeaders(options),
    })
    return requireContractData<SearchProviderTestResult>(body, '测试图片搜索供应商连接失败')
  } catch (error) {
    throw withControlCopy(error, '测试图片搜索供应商连接失败')
  }
}

// withControlCopy is shared — see admin/shared/core/controlCopy.ts.
