import adminAccountApi, {
  FALLBACK_ADMIN_ACCOUNT_LIMITS,
  normalizeAdminAccountLimits,
  type AdminAccount,
  type AdminAccountLimits,
  type AdminAccountModel,
  type AdminAccountModelInput,
  type AdminAccountUpdateRequest,
} from '../api/adminAccounts'
import { assertApiOk, requireApiData } from './adminApiResponse'

export {
  FALLBACK_ADMIN_ACCOUNT_LIMITS,
  type AdminAccount,
  type AdminAccountLimits,
  type AdminAccountModel,
  type AdminAccountModelInput,
  type AdminAccountUpdateRequest,
}

export type AdminAccountCreateRequest = Parameters<typeof adminAccountApi.create>[0]

const ADMIN_ACCOUNT_LIST_PAGE_SIZE = 100

export async function fetchAdminAccountLimits(): Promise<AdminAccountLimits> {
  const res = await adminAccountApi.getLimits()
  const data = requireApiData(res, '加载账号限制失败')
  return normalizeAdminAccountLimits(data.limits)
}

export async function fetchAllAdminAccounts(): Promise<AdminAccount[]> {
  const all: AdminAccount[] = []
  let page = 1
  let total = Number.POSITIVE_INFINITY

  while (true) {
    const res = await adminAccountApi.list({
      page,
      page_size: ADMIN_ACCOUNT_LIST_PAGE_SIZE,
    })
    const data = requireApiData(res, '加载账号失败')
    const list = data.list ?? []
    total = data.total ?? total
    all.push(...list)
    if (all.length >= total || list.length === 0) break
    page += 1
  }

  return all
}

export async function createAdminAccountRecord(
  data: AdminAccountCreateRequest,
): Promise<void> {
  const res = await adminAccountApi.create(data)
  assertApiOk(res, '创建账号失败')
}

export async function updateAdminAccountRecord(
  id: number,
  data: AdminAccountUpdateRequest,
): Promise<void> {
  const res = await adminAccountApi.update(id, data)
  assertApiOk(res, '更新账号失败')
}

export async function deleteAdminAccountRecord(id: number): Promise<void> {
  const res = await adminAccountApi.delete(id)
  assertApiOk(res, '删除账号失败')
}
