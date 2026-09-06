import {
  banAdminUser as requestBanAdminUser,
  deleteAdminUser as requestDeleteAdminUser,
  impersonateAdminUser as requestImpersonateAdminUser,
  revokeAdminUserSessions as requestRevokeAdminUserSessions,
  unbanAdminUser as requestUnbanAdminUser,
  updateAdminUserRole as requestUpdateAdminUserRole,
} from '../../../shared/api/generated-query/admin-users/admin-users'
import { resolveApiOk } from '../../../shared/api/apiResponse'

export async function banAdminUser(input: {
  publicId: string
  reason?: string
  expires_in_days?: number
}): Promise<void> {
  await resolveApiOk(
    requestBanAdminUser(input.publicId, {
      reason: input.reason,
      expires_in_days: input.expires_in_days,
    }),
    '封禁失败',
  )
}

export async function unbanAdminUser(publicId: string): Promise<void> {
  await resolveApiOk(requestUnbanAdminUser(publicId), '解封失败')
}

export async function revokeAdminUserSessions(publicId: string): Promise<void> {
  await resolveApiOk(requestRevokeAdminUserSessions(publicId), '吊销会话失败')
}

export async function updateAdminUserRole(input: {
  publicId: string
  role: 'admin' | 'user'
}): Promise<void> {
  await resolveApiOk(
    requestUpdateAdminUserRole(input.publicId, { role: input.role }),
    '角色更新失败',
  )
}

export async function deleteAdminUser(publicId: string): Promise<void> {
  await resolveApiOk(requestDeleteAdminUser(publicId), '注销失败')
}

export async function impersonateAdminUser(publicId: string): Promise<void> {
  await resolveApiOk(requestImpersonateAdminUser(publicId), '伪装登录失败')
}
