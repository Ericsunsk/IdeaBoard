import {
  getAdminRateLimitSettings,
  getAdminRegistrationSetting,
  updateAdminRateLimitSettings,
  updateAdminRegistrationSetting,
} from './generated-query/admin-settings/admin-settings'
import {
  banAdminUser,
  deleteAdminUser,
  listAdminUsers,
  revokeAdminUserSessions,
  unbanAdminUser,
  updateAdminUserRole,
  updateAdminUserStatus,
} from './generated-query/admin-users/admin-users'
import type {
  ListAdminUsersParams,
  RateLimitSetting,
  UpdateAdminUserRoleBodyRole,
} from './generated-query/model'

export const adminApi = {
  getRegistrationSetting: () => getAdminRegistrationSetting(),
  updateRegistrationSetting: (enabled: boolean) => updateAdminRegistrationSetting({ enabled }),

  getRateLimitSettings: () => getAdminRateLimitSettings(),
  updateRateLimitSettings: (data: RateLimitSetting) => updateAdminRateLimitSettings(data),

  getUsers: (params: ListAdminUsersParams) => listAdminUsers(params),
  banUser: (
    id: number,
    data: {
      reason?: string
      expires_in_days?: number
    },
  ) => banAdminUser(id, data),
  unbanUser: (id: number) => unbanAdminUser(id),
  revokeUserSessions: (id: number) => revokeAdminUserSessions(id),
  updateUserStatus: (id: number, status: number) => updateAdminUserStatus(id, { status }),
  updateUserRole: (id: number, role: UpdateAdminUserRoleBodyRole) =>
    updateAdminUserRole(id, { role }),
  deleteUser: (id: number) => deleteAdminUser(id),
}
