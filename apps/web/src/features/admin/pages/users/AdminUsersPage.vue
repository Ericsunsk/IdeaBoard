<template>
  <div class="admin-page users-page">
    <div class="summary-grid">
      <AdminSummaryCard label="总用户" :value="users.length" />
      <AdminSummaryCard label="正常" :value="activeUserCount" />
      <AdminSummaryCard label="封禁" :value="bannedUserCount" />
      <AdminSummaryCard label="系统保留" :value="reservedUserCount" />
    </div>

    <AdminToolbar
      v-model:search="searchKeyword"
      search-placeholder="搜索用户..."
      refresh-label="刷新用户"
      @refresh="loadUsers"
      @search-submit="loadUsers"
    >
      <template #actions>
        <div class="control-pill">
          <span class="control-label">开放注册</span>
          <n-switch
            v-model:value="registrationEnabled"
            :loading="registrationSettingLoading"
            size="small"
            @update:value="handleToggleRegistration"
          />
        </div>

        <n-popover
          trigger="click"
          placement="bottom-end"
          :show-arrow="false"
          :theme-overrides="adminPopoverPanelThemeOverrides"
        >
          <template #trigger>
            <n-button secondary class="admin-icon-btn">
              <template #icon
                ><n-icon><Settings /></n-icon
              ></template>
            </n-button>
          </template>
          <div class="rate-limit-panel">
            <div class="panel-header">
              <div class="panel-title">限流策略</div>
              <div class="panel-desc">API 请求频率全局控制</div>
            </div>
            <div class="panel-body">
              <n-form
                :model="rateLimitForm"
                label-placement="top"
                size="small"
                :show-feedback="false"
              >
                <div class="form-row">
                  <n-form-item label="最大请求 (次)" class="flex-1">
                    <n-input-number
                      v-model:value="rateLimitForm.max_requests"
                      :min="1"
                      placeholder="100"
                    />
                  </n-form-item>
                  <n-form-item label="窗口周期 (秒)" class="flex-1">
                    <n-input-number
                      v-model:value="rateLimitForm.window_seconds"
                      :min="1"
                      placeholder="60"
                    />
                  </n-form-item>
                </div>
                <div class="panel-footer">
                  <n-button
                    type="primary"
                    block
                    :loading="rateLimitSaving"
                    @click="handleSaveRateLimit"
                  >
                    保存配置
                  </n-button>
                </div>
              </n-form>
            </div>
          </div>
        </n-popover>
      </template>
    </AdminToolbar>

    <div class="table-card users-table-card">
      <AdminCardHeader title="用户目录" variant="table">
        <template #extra>
          <n-popover
            placement="bottom-end"
            trigger="click"
            :show-arrow="false"
            :theme-overrides="adminPopoverPanelThemeOverrides"
          >
            <template #trigger>
              <n-button quaternary size="small" class="sort-btn">
                <template #icon
                  ><n-icon><Filter /></n-icon
                ></template>
                Sort
                <span v-if="currentSorts.length > 0" class="sort-count"
                  >({{ currentSorts.length }})</span
                >
              </n-button>
            </template>
            <div class="sort-panel">
              <n-checkbox-group v-model:value="currentSorts">
                <n-space vertical :size="12">
                  <n-checkbox value="role" class="minimal-checkbox">管理员优先</n-checkbox>
                  <n-checkbox value="status" class="minimal-checkbox">正常状态优先</n-checkbox>
                  <n-checkbox
                    value="newest"
                    :disabled="currentSorts.includes('oldest')"
                    class="minimal-checkbox"
                    >最新加入</n-checkbox
                  >
                  <n-checkbox
                    value="oldest"
                    :disabled="currentSorts.includes('newest')"
                    class="minimal-checkbox"
                    >最早加入</n-checkbox
                  >
                </n-space>
              </n-checkbox-group>
            </div>
          </n-popover>
        </template>
      </AdminCardHeader>
      <div class="table-container">
        <n-data-table
          :columns="columns"
          :data="sortedUsers"
          :loading="loading"
          :pagination="false"
          :bordered="false"
          :show-header="false"
          :row-key="(row: User) => row.id"
          class="admin-compact-table"
        />
      </div>
    </div>
  </div>

  <!-- 高级封禁弹窗 -->
  <n-modal
    v-model:show="showBanModal"
    preset="card"
    title="高级访问限制"
    style="
      width: 440px;
      border-radius: 16px;
      background: var(--admin-card);
      border: 1px solid var(--admin-border);
    "
  >
    <div style="color: var(--admin-text-2); margin-bottom: 24px; font-size: 14px; line-height: 1.6">
      您即将对用户
      <strong style="color: var(--admin-text-1)">{{ banUserContext?.username }}</strong>
      实施强制访问限制。该用户将立即断开所有连接，并且在封禁期间无法重新登录。
    </div>
    <n-form :model="banForm" label-placement="top">
      <n-form-item label="封禁时长 (Duration)">
        <n-select
          v-model:value="banForm.expiresInDays"
          :options="[
            { label: '永久封禁 (Permanent)', value: 0 },
            { label: '1 天 (24 小时警告)', value: 1 },
            { label: '7 天 (严重警告)', value: 7 },
            { label: '30 天 (短期隔离)', value: 30 },
          ]"
        />
      </n-form-item>
      <n-form-item label="封禁原因 (内部记录)">
        <n-input
          v-model:value="banForm.reason"
          type="textarea"
          placeholder="例如：发表违规内容、恶意刷量..."
          :rows="3"
        />
      </n-form-item>
      <div style="display: flex; justify-content: flex-end; gap: 12px; margin-top: 16px">
        <n-button quaternary @click="showBanModal = false">取消</n-button>
        <n-button
          type="error"
          :loading="banFormLoading"
          style="border-radius: 6px"
          @click="confirmBanUser"
        >
          确认封禁
        </n-button>
      </div>
    </n-form>
  </n-modal>
</template>

<script setup lang="ts">
import { authClient, extractBetterAuthErrorMessage } from '../../../../shared/auth/client'
import {
  NInput,
  NButton,
  NDataTable,
  NIcon,
  NTag,
  NSpace,
  NSwitch,
  NPopover,
  NInputNumber,
  NForm,
  NFormItem,
  NAvatar,
  NDropdown,
  useMessage,
  useDialog,
  NModal,
  NSelect,
  NCheckboxGroup,
  NCheckbox,
} from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import type { User } from '../../../../shared/types'
import { formatDateOnly } from '../../../../shared/utils/dateTime'
import { formatDisplayErrorMessage } from '../../../../shared/utils/errorMessages'
import {
  getListAdminUsersQueryKey,
  useListAdminUsers,
} from '../../../../shared/api/generated-query/admin-users/admin-users'
import {
  getGetAdminRateLimitSettingsQueryKey,
  getGetAdminRegistrationSettingQueryKey,
  useGetAdminRateLimitSettings,
  useGetAdminRegistrationSetting,
} from '../../../../shared/api/generated-query/admin-settings/admin-settings'
import {
  deleteAdminUser,
  updateAdminUserRole,
  updateAdminUserStatus,
  updateRateLimitSettings,
  updateRegistrationSetting,
} from '../../../../shared/query/adminQueries'
import { Filter, MoreHoriz, Settings } from '@iconoir/vue'
import AdminToolbar from '../../shared/components/AdminToolbar.vue'
import { adminPopoverPanelThemeOverrides } from '../../shared/theme/adminThemeOverrides'

const message = useMessage()
const dialog = useDialog()

const queryClient = useQueryClient()
const userListParams = { page: 1, page_size: 100 } as const
const searchKeyword = ref('')
const appliedSearchKeyword = ref('')
const userQueryParams = computed(() => ({
  ...userListParams,
  keyword: appliedSearchKeyword.value || undefined,
}))
const usersQuery = useListAdminUsers(userQueryParams)
const registrationSettingQuery = useGetAdminRegistrationSetting()
const rateLimitSettingsQuery = useGetAdminRateLimitSettings()

function invalidateAdminUsersQuery() {
  return queryClient.invalidateQueries({ queryKey: getListAdminUsersQueryKey() })
}

function invalidateRegistrationSettingQuery() {
  return queryClient.invalidateQueries({ queryKey: getGetAdminRegistrationSettingQueryKey() })
}

function invalidateRateLimitSettingsQuery() {
  return queryClient.invalidateQueries({ queryKey: getGetAdminRateLimitSettingsQueryKey() })
}

const updateUserStatusMutation = useMutation({
  mutationFn: updateAdminUserStatus,
  onSuccess: () => invalidateAdminUsersQuery(),
})
const updateUserRoleMutation = useMutation({
  mutationFn: updateAdminUserRole,
  onSuccess: () => invalidateAdminUsersQuery(),
})
const deleteUserMutation = useMutation({
  mutationFn: deleteAdminUser,
  onSuccess: () => invalidateAdminUsersQuery(),
})
const saveRateLimitMutation = useMutation({
  mutationFn: updateRateLimitSettings,
  onSuccess: async (settings) => {
    rateLimitForm.value.window_seconds = settings.window_seconds
    rateLimitForm.value.max_requests = settings.max_requests
    await invalidateRateLimitSettingsQuery()
  },
})
const updateRegistrationSettingMutation = useMutation({
  mutationFn: updateRegistrationSetting,
  onSuccess: async (setting) => {
    registrationEnabled.value = setting.enabled
    await invalidateRegistrationSettingQuery()
  },
})

const users = computed<User[]>(() => usersQuery.data.value?.data?.list ?? [])
const loading = computed(() => usersQuery.isLoading.value || usersQuery.isFetching.value)
const currentSorts = ref<string[]>(['role', 'status', 'newest'])

// 后端限流配置状态
const rateLimitForm = ref({
  window_seconds: 60,
  max_requests: 100,
})
const rateLimitSaving = computed(() => saveRateLimitMutation.isPending.value)

// 注册限制状态
const registrationEnabled = ref(true)
const registrationSettingLoading = computed(
  () =>
    registrationSettingQuery.isLoading.value ||
    registrationSettingQuery.isFetching.value ||
    updateRegistrationSettingMutation.isPending.value,
)

// 封禁弹窗状态
const showBanModal = ref(false)
const banFormLoading = ref(false)
const banUserContext = ref<User | null>(null)
const banForm = ref({
  expiresInDays: 0,
  reason: '',
})

watch(
  () => usersQuery.error.value,
  (error) => {
    if (error) message.error(formatDisplayErrorMessage(error, '加载用户失败'))
  },
)

watch(
  () => registrationSettingQuery.error.value || rateLimitSettingsQuery.error.value,
  (error) => {
    if (error) message.error(formatDisplayErrorMessage(error, '加载后台配置失败'))
  },
)

watch(
  () => rateLimitSettingsQuery.data.value,
  (response) => {
    const settings = response?.data
    if (!settings || saveRateLimitMutation.isPending.value) return
    rateLimitForm.value.window_seconds = settings.window_seconds
    rateLimitForm.value.max_requests = settings.max_requests
  },
  { immediate: true },
)

watch(
  () => registrationSettingQuery.data.value,
  (response) => {
    const setting = response?.data
    if (!setting || updateRegistrationSettingMutation.isPending.value) return
    registrationEnabled.value = setting.enabled
  },
  { immediate: true },
)

const columns: DataTableColumns<User> = [
  {
    title: '用户信息',
    key: 'user',
    render: (row) => {
      return h('div', { class: 'user-profile-cell' }, [
        h('div', { class: 'avatar-wrapper' }, [
          h(NAvatar, {
            round: true,
            size: 26,
            src: row.avatar || undefined,
            fallbackSrc: 'https://07akioni.oss-cn-beijing.aliyuncs.com/07akioni.jpeg',
          }),
          h('span', {
            class: ['user-avatar-status', row.status === 1 ? 'is-normal' : 'is-banned'],
          }),
        ]),
        h('div', { style: 'display: flex; flex-direction: column; gap: 0px;' }, [
          h('span', { class: 'user-name' }, row.username),
          h('span', { class: 'user-id' }, `ID: ${row.uid}`),
        ]),
      ])
    },
  },
  {
    title: '角色',
    key: 'role',
    width: 100,
    render: (row) => {
      const isAdmin = row.role === 'admin'
      return h(
        NTag,
        {
          type: isAdmin ? 'error' : 'info',
          size: 'small',
          round: true,
          bordered: false,
          style:
            (isAdmin
              ? 'background: rgba(239, 68, 68, 0.1); color: #f87171;'
              : 'background: var(--admin-surface-muted); color: var(--admin-text-3);') +
            ' border-radius: 99px !important; padding: 0 10px; font-weight: 600; font-size: 11px;',
        },
        { default: () => (isAdmin ? 'Admin' : 'User') },
      )
    },
  },
  {
    title: '加入时间',
    key: 'created_at',
    width: 160,
    render: (row) => {
      return h(
        'span',
        { style: 'color: var(--admin-text-3); font-size: 13px;' },
        row.created_at ? formatDateOnly(row.created_at) : '-',
      )
    },
  },
  {
    title: '操作',
    key: 'actions',
    width: 80,
    align: 'right',
    render: (row) => {
      const isAdmin = row.role === 'admin'
      return h(
        NDropdown,
        {
          trigger: 'click',
          options: [
            {
              label: row.status === 1 ? '用户封禁' : '解除封禁',
              key: 'status',
              disabled: isAdmin,
              props: { style: row.status === 1 ? 'color: #f87171' : 'color: #34d399' },
            },
            { label: '吊销会话', key: 'revoke' },
            { label: '伪装登录', key: 'impersonate' },
            {
              label: isAdmin ? '取消管理' : '设为管理',
              key: 'role',
              props: { style: isAdmin ? 'color: #fbbf24' : 'color: #60a5fa' },
            },
            { type: 'divider', key: 'd1' },
            {
              label: '注销用户',
              key: 'delete',
              disabled: isAdmin,
              props: { style: 'color: #f87171' },
            },
          ],
          onSelect: (key) => handleAction(key, row),
        },
        {
          default: () =>
            h(
              NButton,
              { quaternary: true, circle: true, size: 'small' },
              {
                icon: () => h(NIcon, null, { default: () => h(MoreHoriz) }),
              },
            ),
        },
      )
    },
  },
]

const sortedUsers = computed(() => {
  const kw = searchKeyword.value.trim().toLowerCase()
  const filtered = !kw
    ? users.value
    : users.value.filter(
        (u) =>
          u.username.toLowerCase().includes(kw) ||
          u.uid.toLowerCase().includes(kw) ||
          u.id.toString().includes(kw),
      )

  const withSort = [...filtered]
  withSort.sort((a, b) => {
    if (currentSorts.value.includes('role')) {
      const aRoleRank = a.role === 'admin' ? 0 : 1
      const bRoleRank = b.role === 'admin' ? 0 : 1
      if (aRoleRank !== bRoleRank) return aRoleRank - bRoleRank
    }

    if (currentSorts.value.includes('status') && a.status !== b.status) {
      return b.status - a.status
    }

    const aTime = new Date(a.created_at || 0).getTime()
    const bTime = new Date(b.created_at || 0).getTime()
    if (currentSorts.value.includes('newest') && aTime !== bTime) {
      return bTime - aTime
    }
    if (currentSorts.value.includes('oldest') && aTime !== bTime) {
      return aTime - bTime
    }

    return b.id - a.id
  })

  return withSort
})

function loadUsers() {
  const nextKeyword = searchKeyword.value.trim()
  if (appliedSearchKeyword.value !== nextKeyword) {
    appliedSearchKeyword.value = nextKeyword
    return
  }
  void usersQuery.refetch()
}

async function handleSaveRateLimit() {
  try {
    await saveRateLimitMutation.mutateAsync({
      window_seconds: rateLimitForm.value.window_seconds,
      max_requests: rateLimitForm.value.max_requests,
    })
    message.success('限流配置已保存')
  } catch (error) {
    message.error(formatDisplayErrorMessage(error, '保存失败'))
  }
}

async function handleToggleRegistration(newValue: boolean) {
  const fallbackEnabled = registrationSettingQuery.data.value?.data?.enabled ?? !newValue
  try {
    await updateRegistrationSettingMutation.mutateAsync(newValue)
    message.success(newValue ? '已开放注册' : '已关闭注册')
  } catch (error) {
    registrationEnabled.value = fallbackEnabled
    message.error(formatDisplayErrorMessage(error, '更新注册状态失败'))
  }
}

function handleAction(key: string, row: User) {
  switch (key) {
    case 'status':
      handleToggleStatus(row)
      break
    case 'revoke':
      handleRevokeSessions(row)
      break
    case 'impersonate':
      handleImpersonate(row)
      break
    case 'role':
      handleToggleRole(row)
      break
    case 'delete':
      handleDeleteUser(row)
      break
  }
}

function handleToggleStatus(row: User) {
  if (row.role === 'admin') {
    message.warning('管理员账号不可封禁或解封')
    return
  }

  if (row.status === 1) {
    banUserContext.value = row
    banForm.value = { expiresInDays: 0, reason: '' }
    showBanModal.value = true
  } else {
    dialog.warning({
      title: '确认解除封禁',
      content: `确定要恢复用户 ${row.username} 的访问权限吗？`,
      positiveText: '确认解封',
      negativeText: '取消',
      onPositiveClick: async () => {
        try {
          if (row.auth_user_id) {
            const { error } = await authClient.admin.unbanUser({ userId: row.auth_user_id })
            if (error) throw error
          }
          await updateUserStatusMutation.mutateAsync({ id: row.id, status: 1 })
          message.success('已解除封禁')
        } catch (err: any) {
          message.error(extractBetterAuthErrorMessage(err, '解封失败'))
        }
      },
    })
  }
}

const confirmBanUser = async () => {
  if (!banUserContext.value) return
  const row = banUserContext.value
  banFormLoading.value = true
  try {
    await updateUserStatusMutation.mutateAsync({ id: row.id, status: 0 })
    if (row.auth_user_id) {
      let expiresIn: number | undefined = undefined
      if (banForm.value.expiresInDays > 0) {
        expiresIn = banForm.value.expiresInDays * 24 * 60 * 60
      }
      try {
        const { error } = await authClient.admin.banUser({
          userId: row.auth_user_id,
          banReason: banForm.value.reason || undefined,
          banExpiresIn: expiresIn,
        })
        if (error) throw error
      } catch (authError: any) {
        showBanModal.value = false
        void invalidateAdminUsersQuery()
        message.warning(
          extractBetterAuthErrorMessage(
            authError,
            '已完成业务封禁，但身份侧封禁失败，请稍后重试身份侧操作',
          ),
        )
        return
      }
    }
    message.success(`已高级封禁用户 ${row.username}`)
    showBanModal.value = false
  } catch (err: any) {
    message.error(
      err?.message ||
        formatDisplayErrorMessage(err, extractBetterAuthErrorMessage(err, '封禁失败')),
    )
  } finally {
    banFormLoading.value = false
  }
}

function handleRevokeSessions(row: User) {
  if (!row.auth_user_id) return
  dialog.warning({
    title: '确认踢下线',
    content: `将强制吊销账号 ${row.username} 的所有活跃会话，是否继续？`,
    positiveText: '确认',
    negativeText: '取消',
    onPositiveClick: async () => {
      try {
        const { error } = await authClient.admin.revokeUserSessions({ userId: row.auth_user_id! })
        if (error) throw error
        message.success('已强制吊销所有会话')
      } catch (err: any) {
        message.error(extractBetterAuthErrorMessage(err, '操作失败'))
      }
    },
  })
}

async function handleImpersonate(row: User) {
  if (!row.auth_user_id) {
    message.error('目标用户缺少 Auth 标识，无法伪装登录')
    return
  }
  try {
    const { error } = await authClient.admin.impersonateUser({ userId: row.auth_user_id! })
    if (error) throw error
    message.success(`正在以 ${row.username} 的身份登录...`)
    setTimeout(() => (window.location.href = '/'), 1000)
  } catch (err: any) {
    message.error(extractBetterAuthErrorMessage(err, '伪装失败'))
  }
}

function handleDeleteUser(row: User) {
  dialog.error({
    title: '危险操作：彻底注销',
    content: `确定要彻底删除用户 ${row.username} 吗？此操作将清除该用户在 Auth 及业务数据库中的所有数据，不可撤销！`,
    positiveText: '确定删除',
    negativeText: '取消',
    onPositiveClick: async () => {
      try {
        await deleteUserMutation.mutateAsync(row.id)
        message.success('用户已受控彻底注销')
      } catch (err: any) {
        message.error(formatDisplayErrorMessage(err, '注销失败'))
      }
    },
  })
}

function handleToggleRole(row: User) {
  const nextRole: 'admin' | 'user' = row.role === 'admin' ? 'user' : 'admin'
  dialog.warning({
    title: nextRole === 'admin' ? '确认设为管理员' : '确认取消管理员',
    content:
      nextRole === 'admin'
        ? `确定要将用户 ${row.username} 设为管理员吗？该用户将获得后台访问权限。`
        : `确定要取消用户 ${row.username} 的管理员权限吗？`,
    positiveText: '确认',
    negativeText: '取消',
    onPositiveClick: async () => {
      try {
        await updateUserRoleMutation.mutateAsync({ id: row.id, role: nextRole })
        message.success(nextRole === 'admin' ? '已设为管理员' : '已取消管理员')
      } catch (err: any) {
        message.error(formatDisplayErrorMessage(err, '角色更新失败'))
      }
    },
  })
}

const activeUserCount = computed(() => users.value.filter((u) => u.status === 1).length)
const bannedUserCount = computed(() => users.value.filter((u) => u.status === 0).length)
const reservedUserCount = computed(() => users.value.filter((u) => u.role === 'admin').length)
</script>

<style scoped>
@import '../../shared/styles/adminCommon.css';

.users-page {
  gap: 14px;
}

.admin-compact-table :deep(.user-profile-cell) {
  display: flex;
  align-items: center;
  gap: 12px;
}

.admin-compact-table :deep(.avatar-wrapper) {
  position: relative;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 26px;
  height: 26px;
  overflow: visible;
  flex-shrink: 0;
}

.admin-compact-table :deep(.user-avatar-status) {
  position: absolute;
  right: 0;
  bottom: 0;
  width: 8px;
  height: 8px;
  box-sizing: border-box;
  border: 1.5px solid var(--admin-card-strong);
  border-radius: 50%;
  z-index: 2;
  pointer-events: none;
}

.admin-compact-table :deep(.user-avatar-status.is-normal) {
  background: #10b981;
}

.admin-compact-table :deep(.user-avatar-status.is-banned) {
  background: #ef4444;
}

.admin-compact-table :deep(.user-name) {
  font-weight: 600;
  color: var(--admin-text-1);
  font-size: 14px;
}

.admin-compact-table :deep(.user-id) {
  font-size: 11px;
  color: var(--admin-text-3);
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
}

.admin-compact-table :deep(.status-badge-cell) {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 4px 10px;
  background-color: var(--admin-surface-muted);
  border: 1px solid var(--admin-border);
  border-radius: 999px;
}

.admin-compact-table :deep(.status-text) {
  font-size: 12px;
  font-weight: 500;
  color: var(--admin-text-2);
}

.rate-limit-panel {
  width: 280px;
  background: var(--admin-card);
}

.panel-header {
  padding: 16px 20px;
  border-bottom: 1px solid var(--admin-border);
}

.panel-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--admin-text-1);
}

.panel-desc {
  font-size: 12px;
  color: var(--admin-text-3);
  margin-top: 2px;
}

.panel-body {
  padding: 20px;
}

.form-row {
  display: flex;
  gap: 12px;
}

.flex-1 {
  flex: 1;
}

.panel-footer {
  margin-top: 16px;
}

.users-table-card {
  padding: 0;
  overflow: hidden;
}

.sort-btn {
  border-radius: 6px;
  font-weight: 500;
  color: var(--admin-text-2);
}

.sort-count {
  margin-left: 4px;
  opacity: 0.8;
}

.sort-panel {
  min-width: 130px;
  padding: 12px;
}

.minimal-checkbox {
  --n-font-size: 13px !important;
  color: var(--admin-text-2);
}

.minimal-checkbox:hover {
  color: var(--admin-text-1);
}

.table-container {
  padding: 0;
}

.users-table-card :deep(.n-data-table-thead) {
  display: none;
}
</style>
