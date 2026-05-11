import { getAdminSettings } from './generated/admin-settings/admin-settings'
import type {
  GetAdminRateLimitSettingsResult,
  GetAdminRegistrationSettingResult,
  UpdateAdminRateLimitSettingsResult,
  UpdateAdminRegistrationSettingResult,
} from './generated/admin-settings/admin-settings'
import { getAdminUsers } from './generated/admin-users/admin-users'
import type {
  DeleteAdminUserResult,
  ListAdminUsersResult,
  UpdateAdminUserRoleResult,
  UpdateAdminUserStatusResult,
} from './generated/admin-users/admin-users'

const adminSettingsApi = getAdminSettings()
const adminUsersApi = getAdminUsers()

export const adminApi = {
  getRegistrationSetting: (): Promise<GetAdminRegistrationSettingResult> =>
    adminSettingsApi.getAdminRegistrationSetting(),
  updateRegistrationSetting: (enabled: boolean): Promise<UpdateAdminRegistrationSettingResult> =>
    adminSettingsApi.updateAdminRegistrationSetting({ enabled }),

  getRateLimitSettings: (): Promise<GetAdminRateLimitSettingsResult> =>
    adminSettingsApi.getAdminRateLimitSettings(),
  updateRateLimitSettings: (data: { window_seconds: number; max_requests: number }): Promise<UpdateAdminRateLimitSettingsResult> =>
    adminSettingsApi.updateAdminRateLimitSettings(data),

  getUsers: (params: { keyword?: string; page?: number; page_size?: number }): Promise<ListAdminUsersResult> =>
    adminUsersApi.listAdminUsers(params),
  updateUserStatus: (id: number, status: number): Promise<UpdateAdminUserStatusResult> =>
    adminUsersApi.updateAdminUserStatus(id, { status }),
  updateUserRole: (id: number, role: 'admin' | 'user'): Promise<UpdateAdminUserRoleResult> =>
    adminUsersApi.updateAdminUserRole(id, { role }),
  deleteUser: (id: number): Promise<DeleteAdminUserResult> =>
    adminUsersApi.deleteAdminUser(id),
}
