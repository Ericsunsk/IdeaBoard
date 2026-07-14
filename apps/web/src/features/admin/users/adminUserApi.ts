import {
  banAdminUser as requestBanAdminUser,
  deleteAdminUser as requestDeleteAdminUser,
  revokeAdminUserSessions as requestRevokeAdminUserSessions,
  unbanAdminUser as requestUnbanAdminUser,
  updateAdminUserRole as requestUpdateAdminUserRole,
} from '../../../shared/api/generated-query/admin-users/admin-users'
import { resolveApiOk } from '../../../shared/api/apiResponse'

export async function banAdminUser(input: {
  id: number
  reason?: string
  expires_in_days?: number
}): Promise<void> {
  await resolveApiOk(
    requestBanAdminUser(input.id, {
      reason: input.reason,
      expires_in_days: input.expires_in_days,
    }),
    '封禁失败',
  )
}

export async function unbanAdminUser(id: number): Promise<void> {
  await resolveApiOk(requestUnbanAdminUser(id), '解封失败')
}

export async function revokeAdminUserSessions(id: number): Promise<void> {
  await resolveApiOk(requestRevokeAdminUserSessions(id), '吊销会话失败')
}

export async function updateAdminUserRole(input: {
  id: number
  role: 'admin' | 'user'
}): Promise<void> {
  await resolveApiOk(requestUpdateAdminUserRole(input.id, { role: input.role }), '角色更新失败')
}

export async function deleteAdminUser(id: number): Promise<void> {
  await resolveApiOk(requestDeleteAdminUser(id), '注销失败')
}
