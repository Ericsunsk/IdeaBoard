import {
  banAdminUser as requestBanAdminUser,
  deleteAdminUser as requestDeleteAdminUser,
  revokeAdminUserSessions as requestRevokeAdminUserSessions,
  unbanAdminUser as requestUnbanAdminUser,
  updateAdminUserRole as requestUpdateAdminUserRole,
} from '../../../../shared/api/generated-query/admin-users/admin-users'
import { assertApiOk } from './adminApiResponse'

export async function banAdminUser(input: {
  id: number
  reason?: string
  expires_in_days?: number
}): Promise<void> {
  const res = await requestBanAdminUser(input.id, {
    reason: input.reason,
    expires_in_days: input.expires_in_days,
  })
  assertApiOk(res, '封禁失败')
}

export async function unbanAdminUser(id: number): Promise<void> {
  const res = await requestUnbanAdminUser(id)
  assertApiOk(res, '解封失败')
}

export async function revokeAdminUserSessions(id: number): Promise<void> {
  const res = await requestRevokeAdminUserSessions(id)
  assertApiOk(res, '吊销会话失败')
}

export async function updateAdminUserRole(input: {
  id: number
  role: 'admin' | 'user'
}): Promise<void> {
  const res = await requestUpdateAdminUserRole(input.id, { role: input.role })
  assertApiOk(res, '角色更新失败')
}

export async function deleteAdminUser(id: number): Promise<void> {
  const res = await requestDeleteAdminUser(id)
  assertApiOk(res, '注销失败')
}
