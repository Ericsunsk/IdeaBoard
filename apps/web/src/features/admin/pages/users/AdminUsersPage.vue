<template>
  <AdminTablePanel viewport-height content-class="min-h-0 flex-1 overflow-hidden px-6 pb-6">
    <template #toolbar-start>
      <AdminToolbarSearch v-model="searchKeyword" @enter="loadUsers" @clear="clearSearch" />
      <span :class="adminToolbarMetricClass">
        <span class="h-2.5 w-2.5 rounded-full bg-admin-success"></span>
        <span>{{ totalUsersLabel }}</span>
      </span>
      <Tag :value="`${activeUserCount} 正常`" severity="success" rounded />
      <Tag
        v-if="bannedUserCount > 0"
        :value="`${bannedUserCount} 封禁`"
        severity="danger"
        rounded
      />
      <Tag :value="`${adminUserCount} 管理员`" severity="info" rounded />
      <Tag v-if="selectedUsers.length > 0" :value="`已选 ${selectedUsers.length}`" rounded />
    </template>

    <template #toolbar-end>
      <Button
        text
        :class="[adminToolbarIconButtonClass, hasUserFilter ? '!border-admin-text-1' : '']"
        aria-label="筛选用户"
        :aria-pressed="hasUserFilter"
        @click="toggleUserFilterPanel"
      >
        <template #icon>
          <Filter :class="['size-4', hasUserFilter ? 'text-admin-text-1' : 'text-admin-text-2']" />
        </template>
      </Button>
      <Popover ref="userFilterPopoverRef">
        <div :class="adminFilterPopoverClass">
          <div class="border-b border-admin-border pb-2">
            <div :class="adminPopoverTitleClass">筛选</div>
          </div>

          <label class="flex min-w-0 flex-col gap-1.5 text-admin-meta font-[650] text-admin-text-2">
            <span>角色</span>
            <Select
              v-model="userRoleFilter"
              :options="userRoleFilterOptions"
              option-label="label"
              option-value="value"
              size="small"
              fluid
            />
          </label>

          <label class="flex min-w-0 flex-col gap-1.5 text-admin-meta font-[650] text-admin-text-2">
            <span>状态</span>
            <Select
              v-model="userStatusFilter"
              :options="userStatusFilterOptions"
              option-label="label"
              option-value="value"
              size="small"
              fluid
            />
          </label>

          <Button
            label="重置筛选"
            size="small"
            severity="contrast"
            :disabled="!hasUserFilter"
            @click="resetUserFilters"
          />
        </div>
      </Popover>

      <Button
        text
        :class="adminToolbarIconButtonClass"
        aria-label="用户配置"
        @click="toggleUserConfigPanel"
      >
        <template #icon>
          <Settings class="size-4 text-admin-text-2" />
        </template>
      </Button>
      <Popover ref="userConfigPopoverRef">
        <div :class="adminConfigPopoverClass">
          <div class="border-b border-admin-border pb-2">
            <div :class="adminPopoverTitleClass">用户配置</div>
          </div>

          <div
            class="flex items-center justify-between rounded-lg border border-admin-border bg-admin-surface-muted px-3 py-2"
          >
            <span class="text-admin-caption font-bold text-admin-text-2">开放注册</span>
            <ToggleSwitch
              v-model="registrationEnabled"
              size="small"
              :disabled="registrationSettingLoading"
              @update:model-value="handleToggleRegistration"
            />
          </div>

          <div class="flex flex-col gap-2 rounded-lg border border-admin-border bg-white p-3">
            <div class="flex items-center gap-1.5 text-admin-meta font-bold text-admin-text-2">
              <Settings class="size-4 text-admin-text-3" />
              <span>限流策略</span>
            </div>
            <div class="mt-1 grid grid-cols-1 gap-2.5">
              <label
                class="flex min-w-0 flex-col gap-1 text-admin-meta font-[650] text-admin-text-2"
              >
                <span>最大请求 (次)</span>
                <InputNumber
                  v-model="rateLimitForm.max_requests"
                  :min="1"
                  placeholder="100"
                  size="small"
                  fluid
                />
              </label>
              <label
                class="flex min-w-0 flex-col gap-1 text-admin-meta font-[650] text-admin-text-2"
              >
                <span>窗口周期 (秒)</span>
                <InputNumber
                  v-model="rateLimitForm.window_seconds"
                  :min="1"
                  placeholder="60"
                  size="small"
                  fluid
                />
              </label>
            </div>
            <Button
              label="保存配置"
              size="small"
              class="mt-2"
              :loading="rateLimitSaving"
              fluid
              @click="handleSaveRateLimit"
            />
          </div>
        </div>
      </Popover>

      <div class="h-5 w-[1px] bg-admin-border-soft"></div>

      <Button text :class="adminToolbarIconButtonClass" aria-label="刷新用户" @click="loadUsers">
        <template #icon>
          <Refresh class="size-4 text-admin-text-2" />
        </template>
      </Button>

      <div class="flex items-center gap-1.5">
        <div :class="adminToolbarPageLabelClass">{{ currentPage }} / {{ totalPages }}</div>
        <Button
          text
          :class="adminToolbarPagerButtonClass"
          :disabled="currentPage === 1 || loading"
          aria-label="上一页"
          @click="handlePrevPage"
        >
          <template #icon>
            <NavArrowLeft class="size-4 text-admin-text-2" />
          </template>
        </Button>
        <Button
          text
          :class="adminToolbarPagerButtonClass"
          :disabled="currentPage === totalPages || loading"
          aria-label="下一页"
          @click="handleNextPage"
        >
          <template #icon>
            <NavArrowRight class="size-4 text-admin-text-2" />
          </template>
        </Button>
      </div>
    </template>

    <AdminTableFrame class="h-full">
      <div
        class="app-scrollbar h-full overflow-y-auto overscroll-contain"
        :class="{ 'is-scrolling': isUserTableScrolling }"
        @scroll.passive="handleUserTableScroll"
      >
        <DataTable
          v-model:selection="selectedUsers"
          :value="users"
          :loading="loading"
          data-key="id"
          size="small"
          lazy
          scrollable
          :pt="adminDenseDataTablePt"
          :table-style="{ minWidth: '920px' }"
        >
          <Column
            selection-mode="multiple"
            header-style="width: 3.5rem"
            :body-style="{ width: '3.5rem' }"
          />

          <Column field="username" header="用户" :style="{ minWidth: '260px' }">
            <template #body="{ data: user }">
              <div class="flex min-w-0 items-center gap-3">
                <div class="relative shrink-0">
                  <Avatar
                    v-if="user.avatar"
                    :image="user.avatar"
                    shape="circle"
                    class="!h-9 !w-9"
                  />
                  <Avatar
                    v-else
                    :label="getUserInitial(user)"
                    shape="circle"
                    class="!h-9 !w-9 !bg-admin-surface-muted !font-bold !text-admin-accent"
                  />
                  <span
                    class="absolute top-0 right-0 h-2.5 w-2.5 rounded-full border-2 border-admin-card-strong"
                    :class="user.status === 1 ? 'bg-admin-success' : 'bg-admin-danger'"
                  />
                </div>
                <div class="min-w-0">
                  <div class="truncate text-admin-content font-bold text-admin-text-1">
                    {{ user.username }}
                  </div>
                  <div class="mt-0.5 truncate font-mono text-admin-caption text-admin-text-3">
                    {{ user.uid }}
                  </div>
                </div>
              </div>
            </template>
          </Column>

          <Column field="role" header="角色" :style="{ width: '124px' }">
            <template #body="{ data: user }">
              <Tag
                :value="user.role === 'admin' ? 'Admin' : 'User'"
                :severity="user.role === 'admin' ? 'danger' : 'secondary'"
                rounded
              />
            </template>
          </Column>

          <Column header="认证 ID" :style="{ minWidth: '180px' }">
            <template #body="{ data: user }">
              <span
                class="block max-w-[168px] truncate font-mono text-admin-caption text-admin-text-3"
                :title="user.auth_user_id || '-'"
              >
                {{ user.auth_user_id || '-' }}
              </span>
            </template>
          </Column>

          <Column field="created_at" header="加入时间" :style="{ width: '136px' }">
            <template #body="{ data: user }">
              <span class="text-admin-body text-admin-text-3">
                {{ user.created_at ? formatDateOnly(user.created_at) : '-' }}
              </span>
            </template>
          </Column>

          <Column field="status" header="状态" :style="{ width: '112px' }">
            <template #body="{ data: user }">
              <Tag
                :value="user.status === 1 ? '正常' : '封禁'"
                :severity="user.status === 1 ? 'success' : 'danger'"
                rounded
              />
            </template>
          </Column>

          <Column header="更多" :style="{ width: '72px' }">
            <template #body="{ data: user }">
              <Button
                text
                :class="adminRoundIconButtonClass"
                aria-haspopup="dialog"
                :aria-label="`${user.username} 操作菜单`"
                @click="toggleUserActionMenu($event, user)"
              >
                <template #icon>
                  <Search class="size-4 text-admin-text-2" />
                </template>
              </Button>
            </template>
          </Column>

          <template #empty>
            <div class="py-8 text-center text-admin-body text-admin-text-3">
              {{ emptyUserMessage }}
            </div>
          </template>
          <template #loading>正在加载用户...</template>
        </DataTable>
        <div
          v-if="userPlaceholderRows > 0"
          class="pointer-events-none min-w-[920px]"
          aria-hidden="true"
        >
          <div
            v-for="index in userPlaceholderRows"
            :key="index"
            class="h-[56px] border-t border-admin-border-soft bg-white"
          ></div>
        </div>
      </div>
    </AdminTableFrame>
    <Popover
      ref="userActionPopoverRef"
      :pt="userActionPopoverPt"
      @hide="handleUserActionPopoverHide"
    >
      <div class="flex min-w-[176px] flex-col gap-1">
        <template v-for="item in userActionItems" :key="item.key">
          <div v-if="item.divided" class="my-1 h-px bg-admin-border-soft" aria-hidden="true" />
          <Button
            :label="item.label"
            text
            :severity="item.key === 'delete' ? 'danger' : 'secondary'"
            :disabled="item.disabled"
            class="!w-full !justify-start !rounded-md !px-3 !py-2.5 !text-admin-meta !font-[650]"
            @click="handleActionSelect(item.key)"
          />
        </template>
      </div>
    </Popover>
  </AdminTablePanel>

  <Dialog
    :visible="showBanModal"
    modal
    dismissable-mask
    :draggable="false"
    :style="{ width: '480px', maxWidth: 'calc(100vw - 24px)' }"
    @update:visible="handleBanDialogVisibleUpdate"
  >
    <template #header>
      <div class="min-w-0 pr-4">
        <div class="mb-3 flex flex-wrap gap-2">
          <Tag value="访问控制" severity="secondary" rounded />
          <Tag value="强制限制" severity="danger" rounded />
        </div>
        <h2 class="m-0 text-admin-modal-title leading-[1.15] font-bold text-admin-title-color">
          高级访问限制
        </h2>
        <p class="mt-2.5 mb-0 text-admin-content leading-7 text-admin-subtitle-color">
          {{ banModalDescription }}
        </p>
      </div>
    </template>

    <div class="flex flex-col gap-3.5">
      <label class="flex min-w-0 flex-col gap-1.5 text-admin-meta font-[650] text-admin-text-2">
        <span>封禁时长 (Duration)</span>
        <Select
          v-model="banForm.expiresInDays"
          :options="banDurationOptions"
          option-label="label"
          option-value="value"
          class="w-full"
        />
      </label>
      <label class="flex min-w-0 flex-col gap-1.5 text-admin-meta font-[650] text-admin-text-2">
        <span>封禁原因 (内部记录)</span>
        <Textarea
          v-model="banForm.reason"
          placeholder="例如：发表违规内容、恶意刷量..."
          rows="3"
          auto-resize
          class="w-full"
        />
      </label>
    </div>

    <template #footer>
      <div class="flex w-full flex-col gap-3 md:flex-row md:items-end md:justify-between">
        <div class="max-w-[480px] text-admin-meta leading-6 text-admin-text-3">
          封禁会立即断开该用户所有连接，封禁期间无法重新登录。
        </div>
        <div class="flex flex-wrap justify-end gap-2">
          <Button label="取消" severity="secondary" outlined @click="closeBanModal" />
          <Button
            label="确认封禁"
            severity="danger"
            :loading="banFormLoading"
            @click="confirmBanUser"
          />
        </div>
      </div>
    </template>
  </Dialog>
</template>

<script setup lang="ts">
import { authClient, extractBetterAuthErrorMessage } from '../../../../shared/auth/client'
import Avatar from 'primevue/avatar'
import Button from 'primevue/button'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import Dialog from 'primevue/dialog'
import InputNumber from 'primevue/inputnumber'
import Popover from 'primevue/popover'
import type { PopoverPassThroughOptions } from 'primevue/popover'
import Select from 'primevue/select'
import Tag from 'primevue/tag'
import Textarea from 'primevue/textarea'
import ToggleSwitch from 'primevue/toggleswitch'
import { useConfirm } from 'primevue/useconfirm'
import type { User } from '../../../../shared/types'
import { useAppToast } from '../../../../shared/composables/useAppToast'
import { formatDateOnly } from '../../../../shared/utils/dateTime'
import { formatDisplayErrorMessage } from '../../../../shared/utils/errorMessages'
import {
  getListAdminUsersQueryKey,
  useListAdminUsers,
} from '../../../../shared/api/generated-query/admin-users/admin-users'
import type { ListAdminUsersParams } from '../../../../shared/api/generated-query/model'
import {
  getGetAdminRateLimitSettingsQueryKey,
  getGetAdminRegistrationSettingQueryKey,
  useGetAdminRateLimitSettings,
  useGetAdminRegistrationSetting,
} from '../../../../shared/api/generated-query/admin-settings/admin-settings'
import {
  banAdminUser,
  deleteAdminUser,
  revokeAdminUserSessions,
  unbanAdminUser,
  updateAdminUserRole,
  updateRateLimitSettings,
  updateRegistrationSetting,
} from '../../shared/queries/adminActions'
import AdminToolbarSearch from '../../shared/components/AdminToolbarSearch.vue'
import AdminTableFrame from '../../shared/components/AdminTableFrame.vue'
import AdminTablePanel from '../../shared/components/AdminTablePanel.vue'
import { useTransientScrolling } from '../../shared/composables/useTransientScrolling'
import { adminDenseDataTablePt } from '../../shared/lib/adminDataTablePt'
import {
  adminConfigPopoverClass,
  adminFilterPopoverClass,
  adminPopoverTitleClass,
  adminRoundIconButtonClass,
  adminToolbarIconButtonClass,
  adminToolbarMetricClass,
  adminToolbarPageLabelClass,
  adminToolbarPagerButtonClass,
} from '../../shared/lib/adminUiClasses'
import { Filter, NavArrowLeft, NavArrowRight, Refresh, Search, Settings } from '@iconoir/vue'

type UserActionKey = 'status' | 'revoke' | 'impersonate' | 'role' | 'delete'

type UserActionItem = {
  key: UserActionKey
  label: string
  disabled?: boolean
  divided?: boolean
}

type PrimePopupPanel = {
  toggle: (event: Event) => void
  hide?: () => void
}

type UserRoleFilter = 'all' | 'admin' | 'user'
type UserStatusFilter = 'all' | 'active' | 'banned'

const appToast = useAppToast()
const confirm = useConfirm()
const userActionPopoverPt = {
  root: {
    class: '!rounded-lg !border-admin-border !bg-admin-card !shadow-none',
  },
  content: { class: '!p-1.5' },
} satisfies PopoverPassThroughOptions

const queryClient = useQueryClient()
const userFilterPopoverRef = ref<PrimePopupPanel | null>(null)
const userConfigPopoverRef = ref<PrimePopupPanel | null>(null)
const userActionPopoverRef = ref<PrimePopupPanel | null>(null)
const activeActionUser = ref<User | null>(null)
const selectedUsers = ref<User[]>([])
const { isScrolling: isUserTableScrolling, handleScroll: handleUserTableScroll } =
  useTransientScrolling()

const currentPage = ref(1)
const pageSize = ref(10)
const searchKeyword = ref('')
const appliedSearchKeyword = ref('')
const userRoleFilter = ref<UserRoleFilter>('all')
const userStatusFilter = ref<UserStatusFilter>('all')

const userRoleFilterOptions: Array<{ label: string; value: UserRoleFilter }> = [
  { label: '全部角色', value: 'all' },
  { label: '管理员', value: 'admin' },
  { label: '普通用户', value: 'user' },
]
const userStatusFilterOptions: Array<{ label: string; value: UserStatusFilter }> = [
  { label: '全部状态', value: 'all' },
  { label: '正常', value: 'active' },
  { label: '封禁', value: 'banned' },
]

const hasUserFilter = computed(
  () => userRoleFilter.value !== 'all' || userStatusFilter.value !== 'all',
)
const hasAppliedUserQuery = computed(
  () => Boolean(appliedSearchKeyword.value) || hasUserFilter.value,
)
const selectedUserRole = computed<ListAdminUsersParams['role']>(() =>
  userRoleFilter.value === 'all' ? undefined : userRoleFilter.value,
)
const selectedUserStatus = computed<ListAdminUsersParams['status']>(() => {
  if (userStatusFilter.value === 'all') return undefined
  return userStatusFilter.value === 'active' ? 1 : 0
})

function toggleUserFilterPanel(event: Event) {
  userFilterPopoverRef.value?.toggle(event)
}

function toggleUserConfigPanel(event: Event) {
  userConfigPopoverRef.value?.toggle(event)
}

function resetUserFilters() {
  userRoleFilter.value = 'all'
  userStatusFilter.value = 'all'
}

function handlePrevPage() {
  if (currentPage.value > 1) {
    currentPage.value--
  }
}

function handleNextPage() {
  if (currentPage.value < totalPages.value) {
    currentPage.value++
  }
}

const userQueryParams = computed<ListAdminUsersParams>(() => ({
  page: currentPage.value,
  page_size: pageSize.value,
  keyword: appliedSearchKeyword.value || undefined,
  role: selectedUserRole.value,
  status: selectedUserStatus.value,
}))
const usersQuery = useListAdminUsers(userQueryParams)

const totalRecords = computed(() => usersQuery.data.value?.data?.total ?? 0)
const activeUserCount = computed(() => usersQuery.data.value?.data?.active_count ?? 0)
const bannedUserCount = computed(() => usersQuery.data.value?.data?.banned_count ?? 0)
const adminUserCount = computed(() => usersQuery.data.value?.data?.admin_count ?? 0)
const totalUsersLabel = computed(() =>
  hasAppliedUserQuery.value ? `${totalRecords.value} 匹配用户` : `${totalRecords.value} 用户`,
)
const totalPages = computed(() => Math.ceil(totalRecords.value / pageSize.value) || 1)
const emptyUserMessage = computed(() => (hasAppliedUserQuery.value ? '暂无匹配用户' : '暂无用户'))

watch(totalPages, (newTotal) => {
  if (currentPage.value > newTotal) {
    currentPage.value = Math.max(1, newTotal)
  }
})

function clearSearch() {
  searchKeyword.value = ''
  if (appliedSearchKeyword.value) {
    appliedSearchKeyword.value = ''
  }
  currentPage.value = 1
}

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

const banUserMutation = useMutation({
  mutationFn: banAdminUser,
  onSuccess: () => invalidateAdminUsersQuery(),
})
const unbanUserMutation = useMutation({
  mutationFn: unbanAdminUser,
  onSuccess: () => invalidateAdminUsersQuery(),
})
const revokeUserSessionsMutation = useMutation({
  mutationFn: revokeAdminUserSessions,
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
const userPlaceholderRows = computed(() => {
  if (loading.value || users.value.length === 0) return 0
  return Math.max(0, pageSize.value - users.value.length)
})
const userActionItems = computed<UserActionItem[]>(() => {
  const user = activeActionUser.value
  if (!user) return []

  return getUserActionItems(user)
})

const rateLimitForm = ref({
  window_seconds: 60,
  max_requests: 100,
})
const rateLimitSaving = computed(() => saveRateLimitMutation.isPending.value)

const registrationEnabled = ref(true)
const registrationSettingLoading = computed(
  () =>
    registrationSettingQuery.isLoading.value ||
    registrationSettingQuery.isFetching.value ||
    updateRegistrationSettingMutation.isPending.value,
)

const showBanModal = ref(false)
const banFormLoading = ref(false)
const banUserContext = ref<User | null>(null)
const banForm = ref({
  expiresInDays: 0,
  reason: '',
})

const banDurationOptions: Array<{ label: string; value: number }> = [
  { label: '永久封禁 (Permanent)', value: 0 },
  { label: '1 天 (24 小时警告)', value: 1 },
  { label: '7 天 (严重警告)', value: 7 },
  { label: '30 天 (短期隔离)', value: 30 },
]

const banModalDescription = computed(() =>
  banUserContext.value
    ? `您即将对用户 ${banUserContext.value.username} 实施强制访问限制。`
    : '请选择用户后再配置访问限制。',
)

watch(
  () => usersQuery.error.value,
  (error) => {
    if (error) appToast.error(formatDisplayErrorMessage(error, '加载用户失败'))
  },
)

watch(
  () => registrationSettingQuery.error.value || rateLimitSettingsQuery.error.value,
  (error) => {
    if (error) appToast.error(formatDisplayErrorMessage(error, '加载后台配置失败'))
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

watch([userRoleFilter, userStatusFilter], () => {
  currentPage.value = 1
})

watch(userQueryParams, () => {
  selectedUsers.value = []
})

function getUserInitial(user: User) {
  return (user.username || user.uid || '?').slice(0, 1).toUpperCase()
}

function toggleUserActionMenu(event: Event, user: User) {
  activeActionUser.value = user
  userActionPopoverRef.value?.toggle(event)
}

function handleUserActionPopoverHide() {
  activeActionUser.value = null
}

function getUserActionItems(row: User): UserActionItem[] {
  const isAdmin = row.role === 'admin'
  const hasAuthIdentity = Boolean(row.auth_user_id)
  return [
    {
      key: 'status',
      label: row.status === 1 ? '用户封禁' : '解除封禁',
      disabled: isAdmin,
    },
    { key: 'revoke', label: '吊销会话', disabled: !hasAuthIdentity },
    { key: 'impersonate', label: '伪装登录', disabled: !hasAuthIdentity },
    {
      key: 'role',
      label: isAdmin ? '取消管理' : '设为管理',
    },
    {
      key: 'delete',
      label: '注销用户',
      disabled: isAdmin,
      divided: true,
    },
  ]
}

function handleActionSelect(key: UserActionKey) {
  const row = activeActionUser.value
  if (!row) return

  activeActionUser.value = null
  userActionPopoverRef.value?.hide?.()
  handleAction(key, row)
}

function closeBanModal() {
  if (banFormLoading.value) return
  showBanModal.value = false
  banUserContext.value = null
}

function handleBanDialogVisibleUpdate(visible: boolean) {
  if (!visible) closeBanModal()
}

function loadUsers() {
  const nextKeyword = searchKeyword.value.trim()
  if (appliedSearchKeyword.value !== nextKeyword) {
    currentPage.value = 1
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
    appToast.success('限流配置已保存')
  } catch (error) {
    appToast.error(formatDisplayErrorMessage(error, '保存失败'))
  }
}

async function handleToggleRegistration(newValue: boolean) {
  const fallbackEnabled = registrationSettingQuery.data.value?.data?.enabled ?? !newValue
  try {
    await updateRegistrationSettingMutation.mutateAsync(newValue)
    appToast.success(newValue ? '已开放注册' : '已关闭注册')
  } catch (error) {
    registrationEnabled.value = fallbackEnabled
    appToast.error(formatDisplayErrorMessage(error, '更新注册状态失败'))
  }
}

function handleAction(key: UserActionKey, row: User) {
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

function requireAdminConfirm(options: {
  header: string
  message: string
  acceptLabel: string
  severity: 'warn' | 'danger'
  accept: () => void | Promise<void>
}) {
  confirm.require({
    header: options.header,
    message: options.message,
    rejectLabel: '取消',
    acceptLabel: options.acceptLabel,
    acceptProps: {
      severity: options.severity,
    },
    rejectProps: {
      severity: 'secondary',
      outlined: true,
    },
    accept: () => {
      void options.accept()
    },
  })
}

function handleToggleStatus(row: User) {
  if (row.role === 'admin') {
    appToast.warning('管理员账号不可封禁或解封')
    return
  }

  if (row.status === 1) {
    banUserContext.value = row
    banForm.value = { expiresInDays: 0, reason: '' }
    showBanModal.value = true
  } else {
    requireAdminConfirm({
      header: '确认解除封禁',
      message: `确定要恢复用户 ${row.username} 的访问权限吗？`,
      acceptLabel: '确认解封',
      severity: 'warn',
      accept: async () => {
        try {
          await unbanUserMutation.mutateAsync(row.id)
          appToast.success('已解除封禁')
        } catch (error) {
          appToast.error(formatDisplayErrorMessage(error, '解封失败'))
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
    await banUserMutation.mutateAsync({
      id: row.id,
      reason: banForm.value.reason || undefined,
      expires_in_days: banForm.value.expiresInDays || undefined,
    })
    appToast.success(`已高级封禁用户 ${row.username}`)
    showBanModal.value = false
    banUserContext.value = null
  } catch (error) {
    appToast.error(formatDisplayErrorMessage(error, '封禁失败'))
  } finally {
    banFormLoading.value = false
  }
}

function handleRevokeSessions(row: User) {
  if (!row.auth_user_id) {
    appToast.warning('目标用户缺少 Auth 标识，无法吊销会话')
    return
  }
  requireAdminConfirm({
    header: '确认踢下线',
    message: `将强制吊销账号 ${row.username} 的所有活跃会话，是否继续？`,
    acceptLabel: '确认',
    severity: 'warn',
    accept: async () => {
      try {
        await revokeUserSessionsMutation.mutateAsync(row.id)
        appToast.success('已强制吊销所有会话')
      } catch (error) {
        appToast.error(formatDisplayErrorMessage(error, '操作失败'))
      }
    },
  })
}

async function handleImpersonate(row: User) {
  if (!row.auth_user_id) {
    appToast.error('目标用户缺少 Auth 标识，无法伪装登录')
    return
  }
  try {
    const { error } = await authClient.admin.impersonateUser({ userId: row.auth_user_id! })
    if (error) throw error
    appToast.success(`正在以 ${row.username} 的身份登录...`)
    setTimeout(() => (window.location.href = '/'), 1000)
  } catch (err: any) {
    appToast.error(extractBetterAuthErrorMessage(err, '伪装失败'))
  }
}

function handleDeleteUser(row: User) {
  requireAdminConfirm({
    header: '危险操作：彻底注销',
    message: `确定要彻底删除用户 ${row.username} 吗？此操作将清除该用户在 Auth 及业务数据库中的所有数据，不可撤销！`,
    acceptLabel: '确定删除',
    severity: 'danger',
    accept: async () => {
      try {
        await deleteUserMutation.mutateAsync(row.id)
        appToast.success('用户已受控彻底注销')
      } catch (err: any) {
        appToast.error(formatDisplayErrorMessage(err, '注销失败'))
      }
    },
  })
}

function handleToggleRole(row: User) {
  const nextRole: 'admin' | 'user' = row.role === 'admin' ? 'user' : 'admin'
  requireAdminConfirm({
    header: nextRole === 'admin' ? '确认设为管理员' : '确认取消管理员',
    message:
      nextRole === 'admin'
        ? `确定要将用户 ${row.username} 设为管理员吗？该用户将获得后台访问权限。`
        : `确定要取消用户 ${row.username} 的管理员权限吗？`,
    acceptLabel: '确认',
    severity: 'warn',
    accept: async () => {
      try {
        await updateUserRoleMutation.mutateAsync({ id: row.id, role: nextRole })
        appToast.success(nextRole === 'admin' ? '已设为管理员' : '已取消管理员')
      } catch (err: any) {
        appToast.error(formatDisplayErrorMessage(err, '角色更新失败'))
      }
    },
  })
}
</script>
