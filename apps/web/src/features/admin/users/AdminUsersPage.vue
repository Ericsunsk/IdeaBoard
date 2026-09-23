<template>
  <AdminCollectionPageShell
    :current-page="currentPage"
    :total-pages="totalPages"
    :prev-disabled="!canPrevious"
    :next-disabled="!canNext"
    @prev="previous"
    @next="next"
  >
    <template #toolbar-start>
      <AdminToolbarSearch
        v-model="searchKeyword"
        input-label="搜索用户"
        placeholder="搜索用户姓名、邮箱、IP..."
        class="w-full sm:w-80"
        @enter="loadUsers"
        @clear="clearSearch"
      />
    </template>

    <template #toolbar-end>
      <AdminButton
        title="OAuth Provider 设置"
        variant="ghost"
        size="sm"
        :class="adminFramedIconButtonClass"
        aria-label="OAuth Provider 设置"
        aria-haspopup="dialog"
        @click="oauthSettingsVisible = true"
      >
        <Settings class="size-4 text-admin-text-2" />
      </AdminButton>

      <AdminFilterPopover :active="hasUserFilter" filter-label="筛选用户" @reset="resetUserFilters">
        <label class="flex min-w-0 flex-col gap-1.5 text-admin-meta font-[650] text-admin-text-2">
          <span>角色</span>
          <AdminSelect
            v-model="userRoleFilter"
            :options="userRoleFilterOptions"
            option-label="label"
            option-value="value"
            size="small"
          />
        </label>

        <label class="flex min-w-0 flex-col gap-1.5 text-admin-meta font-[650] text-admin-text-2">
          <span>状态</span>
          <AdminSelect
            v-model="userStatusFilter"
            :options="userStatusFilterOptions"
            option-label="label"
            option-value="value"
            size="small"
          />
        </label>
      </AdminFilterPopover>

      <AdminButton
        variant="ghost"
        :class="adminFramedIconButtonClass"
        aria-label="刷新用户"
        :loading="loading"
        @click="loadUsers"
      >
        <Refresh class="size-4 text-admin-text-2" />
      </AdminButton>
    </template>

    <AdminTable
      :value="users"
      :loading="loading"
      data-key="publicId"
      scrollable
      :frame-class="adminTableFrameClass"
      :table-style="adminTableStyle"
      :class="adminTableClass"
    >
      <template #empty>
        <AdminEmptyState :title="emptyUserMessage" />
      </template>

      <!-- 用户列：头像 + 姓名 -->
      <AdminColumn field="displayName" header="用户" :style="{ minWidth: '240px' }">
        <template #body="{ data: user }">
          <div class="flex items-center gap-2.5 font-sans">
            <div class="relative shrink-0">
              <img
                v-if="user.avatarUrl"
                :src="user.avatarUrl"
                :alt="user.displayName"
                class="size-9 rounded-full object-cover"
              />
              <div
                v-else
                class="flex size-9 items-center justify-center rounded-full bg-admin-surface-muted font-bold text-admin-accent"
              >
                {{ user.initial }}
              </div>
              <span
                class="absolute top-0 right-0 h-2.5 w-2.5 rounded-full border-2 border-admin-card-strong"
                :class="user.isActive ? 'bg-admin-success' : 'bg-admin-danger'"
              />
            </div>
            <span class="truncate font-medium text-admin-text-1">
              {{ user.displayName }}
            </span>
          </div>
        </template>
      </AdminColumn>

      <!-- Email 列：登录渠道图标 + 邮箱 -->
      <AdminColumn field="email" header="Email" :style="{ minWidth: '240px' }">
        <template #body="{ data: user }">
          <div class="flex min-w-0 items-center gap-2 font-mono text-xs text-admin-text-1">
            <span
              v-for="provider in user.providers"
              :key="provider"
              :title="getAuthProviderLabel(provider)"
              class="flex size-5 shrink-0 items-center justify-center text-admin-text-3"
              role="img"
              :aria-label="getAuthProviderLabel(provider)"
            >
              <Mail v-if="provider === 'email'" class="size-4" aria-hidden="true" />
              <Google v-else-if="provider === 'google'" class="size-4" aria-hidden="true" />
              <Discord v-else-if="provider === 'discord'" class="size-4" aria-hidden="true" />
              <span v-else class="text-[10px] font-bold uppercase" aria-hidden="true">
                {{ provider.slice(0, 1) }}
              </span>
            </span>
            <span class="truncate text-admin-text-2">{{ user.email || '-' }}</span>
          </div>
        </template>
      </AdminColumn>

      <!-- IP 列：国旗 + IP 地址 -->
      <AdminColumn field="lastLoginIP" header="IP" :style="{ minWidth: '180px' }">
        <template #body="{ data: user }">
          <div class="flex min-w-0 items-center gap-1.5 font-mono text-xs text-admin-text-1">
            <span
              v-if="getCountryFlag(user.lastLoginCountry)"
              :title="getCountryLabel(user.lastLoginCountry)"
              class="shrink-0 text-sm leading-none"
              role="img"
              :aria-label="getCountryLabel(user.lastLoginCountry)"
            >
              {{ getCountryFlag(user.lastLoginCountry) }}
            </span>
            <Globe
              v-else
              :title="getCountryLabel(user.lastLoginCountry)"
              class="size-3.5 shrink-0 text-admin-text-3"
              role="img"
              :aria-label="getCountryLabel(user.lastLoginCountry)"
            />
            <span
              class="whitespace-nowrap text-admin-text-2"
              :title="user.lastLoginIP || undefined"
            >
              {{ user.lastLoginIP || '-' }}
            </span>
          </div>
        </template>
      </AdminColumn>

      <!-- 角色列：语义化 Badge 徽标 -->
      <AdminColumn field="role" header="角色" :style="{ width: '105px' }">
        <template #body="{ data: user }">
          <AdminBadge :value="user.roleLabel" :severity="user.roleSeverity" rounded />
        </template>
      </AdminColumn>

      <!-- 加入时间列：等宽字体日期 -->
      <AdminColumn field="createdDateLabel" header="加入时间" :style="{ width: '130px' }">
        <template #body="{ data: user }">
          <span class="whitespace-nowrap font-mono text-xs text-admin-text-1">
            {{ user.createdDateLabel }}
          </span>
        </template>
      </AdminColumn>

      <!-- 状态列：语义化 Badge 状态 -->
      <AdminColumn field="status" header="状态" :style="{ width: '105px' }">
        <template #body="{ data: user }">
          <AdminBadge :value="user.statusLabel" :severity="user.statusSeverity" rounded />
        </template>
      </AdminColumn>

      <!-- 账户余额列：可交互打开钱包详情 -->
      <AdminColumn field="availableCredits" header="账户余额" :style="{ width: '140px' }">
        <template #body="{ data: user }">
          <button
            type="button"
            class="font-mono text-xs font-semibold text-admin-text-1 hover:text-admin-accent cursor-pointer transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-admin-accent/20 rounded"
            aria-haspopup="dialog"
            :aria-label="`打开 ${user.displayName} 的账户详情`"
            @click="openWalletDialog(user)"
          >
            {{ user.availableCreditsLabel }}
          </button>
        </template>
      </AdminColumn>

      <!-- 更多操作列 (1:1 对齐事件表第 8 列 More) -->
      <AdminColumn header="More" :style="{ width: '64px' }">
        <template #body="{ data: user }">
          <div class="flex items-center justify-start">
            <button
              type="button"
              :class="adminTableRowActionButtonClass"
              aria-haspopup="dialog"
              :aria-label="`${user.displayName} 操作菜单`"
              @click="toggleUserActionMenu($event, user)"
            >
              <MoreHoriz class="size-4" />
            </button>
          </div>
        </template>
      </AdminColumn>
    </AdminTable>

    <template #footer-label>{{ totalUsersLabel }}</template>
  </AdminCollectionPageShell>

  <AdminRowActionsPopover
    ref="userActionPopoverRef"
    :items="userRowActionItems"
    @select="handleActionSelect"
    @hide="clearActiveActionUser"
  />

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

  <AdminUserWalletDialog
    v-if="walletDialogUser"
    v-model:visible="walletDialogVisible"
    :user="walletDialogUser"
    @refresh-users="loadUsers"
  />

  <OAuthProviderSettingsDialog v-model:visible="oauthSettingsVisible" />
</template>

<script setup lang="ts">
import { Discord, Globe, Google, Mail, MoreHoriz, Refresh, Settings } from '@iconoir/vue'
import { computed, ref } from 'vue'
import AdminBadge from '../shared/components/AdminBadge.vue'
import AdminButton from '../shared/components/AdminButton.vue'
import AdminCollectionPageShell from '../shared/components/AdminCollectionPageShell.vue'
import AdminColumn from '../shared/components/AdminColumn.vue'
import AdminEmptyState from '../shared/components/AdminEmptyState.vue'
import AdminFilterPopover from '../shared/components/AdminFilterPopover.vue'
import AdminRowActionsPopover from '../shared/components/AdminRowActionsPopover.vue'
import AdminSelect from '../shared/components/AdminSelect.vue'
import AdminTable from '../shared/components/AdminTable.vue'
import AdminToolbarSearch from '../shared/components/AdminToolbarSearch.vue'
import { useAdminConfirm } from '../shared/composables/useAdminConfirm'
import {
  adminFramedIconButtonClass,
  adminTableClass,
  adminTableFrameClass,
  adminTableRowActionButtonClass,
  adminTableStyle,
} from '../shared/lib/uiClasses'
import { AdminUserWalletDialog } from '../credits/wallets'
import AdminUserBanDialog from './AdminUserBanDialog.vue'
import { banDurationOptions } from './commands'
import { useAdminUsersContext } from './context'
import OAuthProviderSettingsDialog from './OAuthProviderSettingsDialog.vue'
import { useUsersFeedback } from './useUsersFeedback'
import { getCountryFlag, getCountryLabel, type AdminUserRowView, type UserActionKey } from './view'

type ActionPopupPanel = {
  toggle: (event: Event) => void
  hide?: () => void
}

const feedback = useUsersFeedback()
const adminConfirm = useAdminConfirm()
const userActionPopoverRef = ref<ActionPopupPanel | null>(null)
const walletDialogUser = ref<AdminUserRowView | null>(null)
const oauthSettingsVisible = ref(false)
const walletDialogVisible = computed({
  get: () => Boolean(walletDialogUser.value),
  set: (visible: boolean) => {
    if (!visible) walletDialogUser.value = null
  },
})
const adminUsers = useAdminUsersContext({
  feedback,
  confirm: adminConfirm,
})

const {
  pagination,
  searchKeyword,
  userRoleFilter,
  userStatusFilter,
  hasUserFilter,
  users,
  loading,
  totalUsersLabel,
  emptyUserMessage,
  resetUserFilters,
  clearSearch,
  loadUsers,
} = adminUsers.list
const { currentPage, totalPages, canPrevious, canNext, previous, next } = pagination

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
} = adminUsers.commands

const { userRoleFilterOptions, userStatusFilterOptions } = adminUsers.view
const userRowActionItems = computed(() =>
  userActionItems.value.map((item) => ({ ...item, danger: item.key === 'delete' })),
)

function toggleUserActionMenu(event: Event, user: AdminUserRowView) {
  openUserActionMenu(user)
  userActionPopoverRef.value?.toggle(event)
}

function handleActionSelect(key: string) {
  if (!userActionItems.value.some((item) => item.key === key)) return
  selectUserAction(key as UserActionKey)
  userActionPopoverRef.value?.hide?.()
}

function openWalletDialog(user: AdminUserRowView) {
  walletDialogUser.value = user
}

function getAuthProviderLabel(provider: string) {
  if (provider === 'email') return 'Email'
  if (provider === 'google') return 'Google'
  if (provider === 'discord') return 'Discord'
  return provider
}
</script>
