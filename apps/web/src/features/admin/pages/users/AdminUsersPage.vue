<template>
  <div class="min-w-0">
    <section
      class="min-w-0 overflow-hidden rounded-[28px] border border-admin-border-soft bg-admin-card-strong"
    >
      <div class="px-5 pt-5 pb-4 md:px-6 md:pt-6">
        <div class="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
          <div class="min-w-0">
            <div class="mb-2 flex flex-wrap items-center gap-2">
              <Tag :value="`${totalRecords} 用户`" severity="secondary" rounded />
              <Tag :value="`${currentPageActiveUserCount} 正常`" severity="success" rounded />
              <Tag
                v-if="currentPageBannedUserCount > 0"
                :value="`${currentPageBannedUserCount} 封禁`"
                severity="danger"
                rounded
              />
            </div>
            <h1 class="m-0 text-[30px] leading-[1.1] font-[780] text-admin-title-color">
              用户管理
            </h1>
            <p class="mt-2 mb-0 text-admin-content leading-6 text-admin-subtitle-color">
              系统用户、权限角色与访问状态
            </p>
          </div>

          <div class="flex flex-wrap items-center gap-2 lg:justify-end">
            <Tag :value="`${currentPageReservedUserCount} 管理员`" severity="info" rounded />
            <Tag v-if="selectedUsers.length > 0" :value="`已选 ${selectedUsers.length}`" rounded />
          </div>
        </div>

        <div class="mt-9 flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
          <IconField class="relative w-full lg:max-w-[340px]">
            <InputIcon>
              <Search class="size-4 text-admin-text-3" />
            </InputIcon>
            <InputText
              v-model="searchKeyword"
              placeholder="搜索用户"
              fluid
              class="!rounded-xl !border-admin-border"
              @keydown.enter="loadUsers"
            />
            <Button
              v-if="searchKeyword"
              text
              rounded
              class="absolute top-1/2 right-1 !h-8 !w-8 -translate-y-1/2"
              aria-label="清空搜索"
              @click="clearSearch"
            >
              <template #icon>
                <Xmark />
              </template>
            </Button>
          </IconField>

          <div class="flex flex-wrap items-center gap-2 lg:justify-end">
            <div
              class="flex items-center gap-2 rounded-xl border border-admin-border bg-admin-surface-muted px-3 py-2"
            >
              <span class="text-admin-caption font-bold text-admin-text-2">开放注册</span>
              <ToggleSwitch
                v-model="registrationEnabled"
                size="small"
                :disabled="registrationSettingLoading"
                @update:model-value="handleToggleRegistration"
              />
            </div>

            <Button
              text
              class="!h-10 !w-10 !rounded-xl border border-admin-border bg-white hover:bg-admin-surface-muted transition-colors flex items-center justify-center p-0"
              aria-label="打开限流策略"
              @click="toggleRateLimitPanel"
            >
              <template #icon>
                <Settings class="size-4 text-admin-text-2" />
              </template>
            </Button>
            <Popover ref="rateLimitPopoverRef">
              <div class="w-[280px] max-w-[calc(100vw_-_48px)] bg-admin-card">
                <div class="border-b border-admin-border px-5 py-4">
                  <div class="text-admin-content font-semibold text-admin-title-color">
                    限流策略
                  </div>
                  <div class="mt-0.5 text-admin-meta text-admin-subtitle-color">
                    API 请求频率全局控制
                  </div>
                </div>
                <div class="px-5 py-5">
                  <div class="flex flex-col gap-3.5">
                    <div class="grid grid-cols-1 gap-3">
                      <label
                        class="flex min-w-0 flex-col gap-1.5 text-admin-meta font-[650] text-admin-text-2"
                      >
                        <span>最大请求 (次)</span>
                        <InputNumber
                          v-model="rateLimitForm.max_requests"
                          :min="1"
                          placeholder="100"
                          fluid
                        />
                      </label>
                      <label
                        class="flex min-w-0 flex-col gap-1.5 text-admin-meta font-[650] text-admin-text-2"
                      >
                        <span>窗口周期 (秒)</span>
                        <InputNumber
                          v-model="rateLimitForm.window_seconds"
                          :min="1"
                          placeholder="60"
                          fluid
                        />
                      </label>
                    </div>
                    <div class="mt-4">
                      <Button
                        label="保存配置"
                        :loading="rateLimitSaving"
                        fluid
                        @click="handleSaveRateLimit"
                      />
                    </div>
                  </div>
                </div>
              </div>
            </Popover>

            <Button
              text
              class="!h-10 !w-10 !rounded-xl border border-admin-border bg-white hover:bg-admin-surface-muted transition-colors flex items-center justify-center p-0"
              aria-label="刷新用户"
              @click="loadUsers"
            >
              <template #icon>
                <Refresh class="size-4 text-admin-text-2" />
              </template>
            </Button>
          </div>
        </div>
      </div>

      <div class="min-w-0 border-t border-admin-border-soft">
        <DataTable
          v-model:selection="selectedUsers"
          :value="sortedUsers"
          :loading="loading"
          data-key="id"
          size="small"
          row-hover
          lazy
          paginator
          removable-sort
          scrollable
          sort-mode="multiple"
          :pt="adminDenseDataTablePt"
          :multi-sort-meta="userTableSortMeta"
          :first="paginationFirst"
          :rows="pageSize"
          :total-records="totalRecords"
          :rows-per-page-options="pageSizeOptions"
          paginator-template="FirstPageLink PrevPageLink PageLinks NextPageLink LastPageLink RowsPerPageDropdown CurrentPageReport"
          current-page-report-template="共 {totalRecords} 条"
          :table-style="{ minWidth: '920px' }"
          @page="handlePageChange"
          @sort="handleTableSort"
        >
          <Column
            selection-mode="multiple"
            header-style="width: 3.5rem"
            :body-style="{ width: '3.5rem' }"
          />

          <Column field="username" header="用户" sortable :style="{ minWidth: '280px' }">
            <template #body="{ data: user }">
              <div class="flex min-w-0 items-center gap-3">
                <div class="relative shrink-0">
                  <Avatar
                    v-if="user.avatar"
                    :image="user.avatar"
                    shape="circle"
                    class="!h-10 !w-10"
                  />
                  <Avatar
                    v-else
                    :label="getUserInitial(user)"
                    shape="circle"
                    class="!h-10 !w-10 !bg-admin-surface-muted !font-bold !text-admin-accent"
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

          <Column field="role" header="角色" sortable :style="{ width: '124px' }">
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

          <Column field="created_at" header="加入时间" sortable :style="{ width: '136px' }">
            <template #body="{ data: user }">
              <span class="text-admin-body text-admin-text-3">
                {{ user.created_at ? formatDateOnly(user.created_at) : '-' }}
              </span>
            </template>
          </Column>

          <Column field="status" header="状态" sortable :style="{ width: '112px' }">
            <template #body="{ data: user }">
              <Tag
                :value="user.status === 1 ? '正常' : '封禁'"
                :severity="user.status === 1 ? 'success' : 'danger'"
                rounded
              />
            </template>
          </Column>

          <Column header="" :style="{ width: '72px' }" :body-style="{ textAlign: 'right' }">
            <template #body="{ data: user }">
              <Button
                text
                rounded
                class="!h-8 !w-8 !rounded-full border border-admin-border bg-white hover:bg-admin-surface-muted transition-colors flex items-center justify-center"
                aria-haspopup="menu"
                :aria-label="`${user.username} 操作菜单`"
                @click="toggleUserActionMenu($event, user)"
              >
                <template #icon>
                  <MoreHoriz class="size-4 text-admin-text-2" />
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
        <Menu ref="userActionMenuRef" :model="userActionMenuItems" popup />
      </div>
    </section>
  </div>

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
import DataTable, { type DataTableSortEvent } from 'primevue/datatable'
import Dialog from 'primevue/dialog'
import IconField from 'primevue/iconfield'
import InputIcon from 'primevue/inputicon'
import InputNumber from 'primevue/inputnumber'
import InputText from 'primevue/inputtext'
import Menu from 'primevue/menu'
import type { MenuItem } from 'primevue/menuitem'
import Popover from 'primevue/popover'
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
import { adminDenseDataTablePt } from '../../shared/lib/adminDataTablePt'
import { MoreHoriz, Refresh, Search, Settings, Xmark } from '@iconoir/vue'

type UserActionKey = 'status' | 'revoke' | 'impersonate' | 'role' | 'delete'

type UserActionItem = {
  key: UserActionKey
  label: string
  disabled?: boolean
  divided?: boolean
}

type PrimePopupPanel = {
  toggle: (event: Event) => void
}

type PrimePopupMenu = PrimePopupPanel & {
  hide?: () => void
}

type UserTablePageEvent = {
  page: number
  rows: number
}

type UserTableSortMeta = {
  field: string
  order: 1 | -1
}

const appToast = useAppToast()
const confirm = useConfirm()

const queryClient = useQueryClient()
const rateLimitPopoverRef = ref<PrimePopupPanel | null>(null)
const userActionMenuRef = ref<PrimePopupMenu | null>(null)
const activeActionUser = ref<User | null>(null)
const selectedUsers = ref<User[]>([])

const currentPage = ref(1)
const pageSize = ref(10)
const pageSizeOptions = [10, 20, 50]
const searchKeyword = ref('')
const appliedSearchKeyword = ref('')
const userTableSortMeta = ref<UserTableSortMeta[]>([
  { field: 'role', order: 1 },
  { field: 'status', order: -1 },
  { field: 'created_at', order: -1 },
])

const userQueryParams = computed(() => ({
  page: currentPage.value,
  page_size: pageSize.value,
  keyword: appliedSearchKeyword.value || undefined,
}))
const usersQuery = useListAdminUsers(userQueryParams)

const totalRecords = computed(() => usersQuery.data.value?.data?.total ?? 0)
const totalPages = computed(() => Math.ceil(totalRecords.value / pageSize.value) || 1)
const paginationFirst = computed(() => (currentPage.value - 1) * pageSize.value)
const emptyUserMessage = computed(() => (appliedSearchKeyword.value ? '暂无匹配用户' : '暂无用户'))

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

function handlePageChange(event: UserTablePageEvent) {
  currentPage.value = event.page + 1
  pageSize.value = event.rows
}

function handleTableSort(event: DataTableSortEvent) {
  userTableSortMeta.value = normalizeUserTableSortMeta(event)
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
const sortedUsers = computed(() => {
  const meta = userTableSortMeta.value
  if (meta.length <= 0) return users.value

  return [...users.value].sort((a, b) => compareUsersBySortMeta(a, b, meta))
})
const userActionMenuItems = computed<MenuItem[]>(() => {
  const user = activeActionUser.value
  if (!user) return []

  return getUserActionItems(user).flatMap((item) => {
    const menuItem: MenuItem = {
      label: item.label,
      disabled: item.disabled,
      command: () => handleActionSelect(item.key, user),
    }
    return item.divided ? [{ separator: true } as MenuItem, menuItem] : [menuItem]
  })
})

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

function getUserInitial(user: User) {
  return (user.username || user.uid || '?').slice(0, 1).toUpperCase()
}

function toggleRateLimitPanel(event: Event) {
  rateLimitPopoverRef.value?.toggle(event)
}

function toggleUserActionMenu(event: Event, user: User) {
  activeActionUser.value = user
  userActionMenuRef.value?.toggle(event)
}

function getUserActionItems(row: User): UserActionItem[] {
  const isAdmin = row.role === 'admin'
  return [
    {
      key: 'status',
      label: row.status === 1 ? '用户封禁' : '解除封禁',
      disabled: isAdmin,
    },
    { key: 'revoke', label: '吊销会话' },
    { key: 'impersonate', label: '伪装登录' },
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

function handleActionSelect(key: UserActionKey, row: User) {
  activeActionUser.value = null
  userActionMenuRef.value?.hide?.()
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

function normalizeUserTableSortMeta(event: DataTableSortEvent): UserTableSortMeta[] {
  if (event.multiSortMeta?.length) {
    return event.multiSortMeta
      .filter((meta) => typeof meta.field === 'string' && (meta.order === 1 || meta.order === -1))
      .map((meta) => ({
        field: meta.field as string,
        order: meta.order as 1 | -1,
      }))
  }
  if (typeof event.sortField === 'string' && (event.sortOrder === 1 || event.sortOrder === -1)) {
    return [{ field: event.sortField, order: event.sortOrder }]
  }
  return []
}

function compareUsersBySortMeta(a: User, b: User, meta: UserTableSortMeta[]) {
  for (const sort of meta) {
    const result = compareUserSortValue(
      getUserSortValue(a, sort.field),
      getUserSortValue(b, sort.field),
    )
    if (result !== 0) return result * sort.order
  }
  return b.id - a.id
}

function getUserSortValue(user: User, field: string) {
  if (field === 'created_at') return new Date(user.created_at || 0).getTime()
  if (field === 'status') return Number(user.status || 0)
  if (field === 'role') return user.role === 'admin' ? 0 : 1
  if (field === 'username') return user.username || ''
  return ''
}

function compareUserSortValue(a: string | number, b: string | number) {
  if (typeof a === 'number' && typeof b === 'number') return a - b
  return String(a).localeCompare(String(b))
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
  if (!row.auth_user_id) return
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

const currentPageActiveUserCount = computed(() => users.value.filter((u) => u.status === 1).length)
const currentPageBannedUserCount = computed(() => users.value.filter((u) => u.status === 0).length)
const currentPageReservedUserCount = computed(
  () => users.value.filter((u) => u.role === 'admin').length,
)
</script>
