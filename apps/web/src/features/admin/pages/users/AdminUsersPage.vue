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
      <AdminFilterPopover :active="hasUserFilter" filter-label="筛选用户" @reset="resetUserFilters">
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
      </AdminFilterPopover>

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
      <AdminUserConfigPopover
        ref="userConfigPopoverRef"
        v-model:registration-enabled="registrationEnabled"
        v-model:rate-limit-form="rateLimitForm"
        :registration-setting-loading="registrationSettingLoading"
        :rate-limit-saving="rateLimitSaving"
        :max-requests-limit="RATE_LIMIT_MAX_REQUESTS"
        :window-seconds-limit="RATE_LIMIT_MAX_WINDOW_SECONDS"
        @toggle-registration="handleToggleRegistration"
        @save-rate-limit="handleSaveRateLimit"
      />

      <div class="h-5 w-[1px] bg-admin-border-soft"></div>

      <Button text :class="adminToolbarIconButtonClass" aria-label="刷新用户" @click="loadUsers">
        <template #icon>
          <Refresh class="size-4 text-admin-text-2" />
        </template>
      </Button>

      <AdminPagerControls
        :current-page="currentPage"
        :total-pages="totalPages"
        :prev-disabled="currentPage === 1 || loading"
        :next-disabled="currentPage === totalPages || loading"
        @prev="handlePrevPage"
        @next="handleNextPage"
      />
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
                    :class="isActiveUser(user) ? 'bg-admin-success' : 'bg-admin-danger'"
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
                :value="isAdminUser(user) ? 'Admin' : 'User'"
                :severity="isAdminUser(user) ? 'danger' : 'secondary'"
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
                :value="isActiveUser(user) ? '正常' : '封禁'"
                :severity="isActiveUser(user) ? 'success' : 'danger'"
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
      <div :class="userActionPopoverClass">
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

  <AdminUserBanDialog
    v-model:expires-in-days="banForm.expiresInDays"
    v-model:reason="banForm.reason"
    :visible="showBanModal"
    :description="banModalDescription"
    :duration-options="banDurationOptions"
    :loading="banFormLoading"
    @update:visible="handleBanDialogVisibleUpdate"
    @cancel="closeBanModal"
    @confirm="confirmBanUser"
  />
</template>

<script setup lang="ts">
import { Refresh, Search, Settings } from '@iconoir/vue'
import Avatar from 'primevue/avatar'
import Button from 'primevue/button'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import Popover from 'primevue/popover'
import Select from 'primevue/select'
import Tag from 'primevue/tag'
import { useConfirm } from 'primevue/useconfirm'
import { useAppToast } from '../../../../shared/composables/useAppToast'
import type { User } from '../../../../shared/types'
import { formatDateOnly } from '../../../../shared/utils/dateTime'
import { formatDisplayErrorMessage } from '../../../../shared/utils/errorMessages'
import AdminFilterPopover from '../../shared/components/AdminFilterPopover.vue'
import AdminPagerControls from '../../shared/components/AdminPagerControls.vue'
import AdminTableFrame from '../../shared/components/AdminTableFrame.vue'
import AdminTablePanel from '../../shared/components/AdminTablePanel.vue'
import AdminToolbarSearch from '../../shared/components/AdminToolbarSearch.vue'
import { useTransientScrolling } from '../../shared/composables/useTransientScrolling'
import { adminDenseDataTablePt } from '../../shared/lib/adminDataTablePt'
import { createAdminPopoverPt } from '../../shared/lib/adminOverlayPt'
import {
  adminRoundIconButtonClass,
  adminToolbarIconButtonClass,
  adminToolbarMetricClass,
} from '../../shared/lib/adminUiClasses'
import AdminUserBanDialog from './AdminUserBanDialog.vue'
import AdminUserConfigPopover from './AdminUserConfigPopover.vue'
import {
  getUserInitial,
  isActiveUser,
  isAdminUser,
  type UserActionKey,
  userRoleFilterOptions,
  userStatusFilterOptions,
} from './adminUserView'
import {
  banDurationOptions,
  useAdminUserActions,
} from './useAdminUserActions'
import {
  RATE_LIMIT_MAX_REQUESTS,
  RATE_LIMIT_MAX_WINDOW_SECONDS,
  useAdminUserSettings,
} from './useAdminUserSettings'
import { useAdminUsersList } from './useAdminUsersList'

const userActionPopoverClass = 'flex min-w-[176px] flex-col gap-1'
const userActionPopoverPt = createAdminPopoverPt({ contentClass: '!p-1.5' })

type PrimePopupPanel = {
  toggle: (event: Event) => void
  hide?: () => void
}

const appToast = useAppToast()
const confirm = useConfirm()
const userConfigPopoverRef = ref<InstanceType<typeof AdminUserConfigPopover> | null>(null)
const userActionPopoverRef = ref<PrimePopupPanel | null>(null)
const { isScrolling: isUserTableScrolling, handleScroll: handleUserTableScroll } =
  useTransientScrolling()

const {
  currentPage,
  searchKeyword,
  userRoleFilter,
  userStatusFilter,
  selectedUsers,
  hasUserFilter,
  usersQuery,
  users,
  loading,
  activeUserCount,
  bannedUserCount,
  adminUserCount,
  totalUsersLabel,
  totalPages,
  emptyUserMessage,
  userPlaceholderRows,
  resetUserFilters,
  handlePrevPage,
  handleNextPage,
  clearSearch,
  loadUsers,
} = useAdminUsersList()

const {
  rateLimitForm,
  rateLimitSaving,
  registrationEnabled,
  registrationSettingLoading,
  handleSaveRateLimit,
  handleToggleRegistration,
} = useAdminUserSettings({ message: appToast })

const {
  userActionItems,
  showBanModal,
  banFormLoading,
  banForm,
  banModalDescription,
  openUserActionMenu,
  clearActiveActionUser,
  closeBanModal,
  handleBanDialogVisibleUpdate,
  handleActionSelect: selectUserAction,
  confirmBanUser,
} = useAdminUserActions({
  message: appToast,
  confirm: { require: requireAdminConfirm },
})

watch(
  () => usersQuery.error.value,
  (error) => {
    if (error) appToast.error(formatDisplayErrorMessage(error, '加载用户失败'))
  },
)

function toggleUserConfigPanel(event: Event) {
  userConfigPopoverRef.value?.toggle(event)
}

function toggleUserActionMenu(event: Event, user: User) {
  openUserActionMenu(user)
  userActionPopoverRef.value?.toggle(event)
}

function handleUserActionPopoverHide() {
  clearActiveActionUser()
}

function handleActionSelect(key: UserActionKey) {
  selectUserAction(key)
  userActionPopoverRef.value?.hide?.()
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
</script>
