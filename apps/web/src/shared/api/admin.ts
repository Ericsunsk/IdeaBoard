import {
  getAdminRateLimitSettings,
  getAdminRegistrationSetting,
  updateAdminRateLimitSettings,
  updateAdminRegistrationSetting,
} from './generated-query/admin-settings/admin-settings'
import {
  deleteAdminUser,
  listAdminUsers,
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
  updateUserStatus: (id: number, status: number) => updateAdminUserStatus(id, { status }),
  updateUserRole: (id: number, role: UpdateAdminUserRoleBodyRole) =>
    updateAdminUserRole(id, { role }),
  deleteUser: (id: number) => deleteAdminUser(id),
}
