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
        :input-label="USERS_MESSAGES.searchLabel"
        :placeholder="USERS_MESSAGES.searchPlaceholder"
        class="w-full sm:w-80"
        @enter="loadUsers"
        @clear="clearSearch"
      />
    </template>

    <template #toolbar-end>
      <AdminFilterPopover
        :active="hasUserFilter"
        :filter-label="USERS_MESSAGES.filterUsers"
        @reset="resetUserFilters"
      >
        <label
          class="flex min-w-0 flex-col gap-1.5 text-admin-meta font-admin-strong text-muted-foreground"
        >
          <span>{{ USERS_MESSAGES.role }}</span>
          <AdminSelect
            v-model="userRoleFilter"
            :options="userRoleFilterOptions"
            option-label="label"
            option-value="value"
            size="small"
          />
        </label>

        <label
          class="flex min-w-0 flex-col gap-1.5 text-admin-meta font-admin-strong text-muted-foreground"
        >
          <span>{{ USERS_MESSAGES.status }}</span>
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
        :aria-label="USERS_MESSAGES.refreshUsers"
        :loading="loading"
        @click="loadUsers"
      >
        <Refresh class="size-4 text-muted-foreground" />
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
      <AdminColumn
        field="displayName"
        :header="USERS_MESSAGES.userColumn"
        :style="{ minWidth: '240px' }"
      >
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
                class="flex size-9 items-center justify-center rounded-full bg-muted font-bold text-primary"
              >
                {{ user.initial }}
              </div>
              <span
                class="absolute top-0 right-0 h-2.5 w-2.5 rounded-full border-2 border-card"
                :class="user.isActive ? 'bg-success' : 'bg-destructive'"
              />
            </div>
            <span class="truncate font-medium text-foreground" :title="user.displayName">
              {{ user.displayName }}
            </span>
          </div>
        </template>
      </AdminColumn>

      <!-- Email 列：登录渠道图标 + 邮箱 -->
      <AdminColumn field="email" header="Email" :style="{ minWidth: '240px' }">
        <template #body="{ data: user }">
          <AdminMono as="div" class="flex min-w-0 items-center gap-2 text-xs text-foreground">
            <span
              v-for="provider in user.providers"
              :key="provider"
              :title="getAuthProviderLabel(provider)"
              class="flex size-5 shrink-0 items-center justify-center text-muted-foreground"
              role="img"
              :aria-label="getAuthProviderLabel(provider)"
            >
              <Mail v-if="provider === 'email'" class="size-4" aria-hidden="true" />
              <Google v-else-if="provider === 'google'" class="size-4" aria-hidden="true" />
              <Discord v-else-if="provider === 'discord'" class="size-4" aria-hidden="true" />
              <span v-else class="text-admin-micro font-bold uppercase" aria-hidden="true">
                {{ provider.slice(0, 1) }}
              </span>
            </span>
            <span class="truncate text-muted-foreground" :title="user.email || undefined">{{
              emptyPlaceholder(user.email)
            }}</span>
          </AdminMono>
        </template>
      </AdminColumn>

      <!-- IP 列：国旗 + IP 地址 -->
      <AdminColumn field="lastLoginIP" header="IP" :style="{ minWidth: '180px' }">
        <template #body="{ data: user }">
          <AdminMono as="div" class="flex min-w-0 items-center gap-1.5 text-xs text-foreground">
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
              class="size-3.5 shrink-0 text-muted-foreground"
              role="img"
              :aria-label="getCountryLabel(user.lastLoginCountry)"
            />
            <span
              class="whitespace-nowrap text-muted-foreground"
              :title="user.lastLoginIP || undefined"
            >
              {{ emptyPlaceholder(user.lastLoginIP) }}
            </span>
          </AdminMono>
        </template>
      </AdminColumn>

      <!-- 角色列（方案 A：载体统一 AdminStatusPill，非状态徽标亦收口） -->
      <AdminColumn field="role" :header="USERS_MESSAGES.role" :style="{ width: '105px' }">
        <template #body="{ data: user }">
          <AdminStatusPill :label="user.roleLabel" :variant="user.roleVariant" size="small" />
        </template>
      </AdminColumn>

      <!-- 账户余额列：可交互打开钱包详情（范式：属性列，状态列之前） -->
      <AdminColumn
        field="availableCredits"
        :header="USERS_MESSAGES.balanceColumn"
        :style="{ width: '140px' }"
      >
        <template #body="{ data: user }">
          <button
            type="button"
            :class="adminInlineJumpClass"
            aria-haspopup="dialog"
            :aria-label="USERS_MESSAGES.openWalletAria(user.displayName)"
            @click="openWalletDialog(user)"
          >
            {{ user.availableCreditsLabel }}
          </button>
        </template>
      </AdminColumn>

      <!-- 状态列（方案 A：载体统一 AdminStatusPill，正常 success / 封禁 danger） -->
      <AdminColumn field="status" :header="USERS_MESSAGES.status" :style="{ width: '105px' }">
        <template #body="{ data: user }">
          <AdminStatusPill :label="user.statusLabel" :variant="user.statusVariant" size="small" />
        </template>
      </AdminColumn>

      <!-- 加入时间列：等宽字体日期（范式：时间列，More 操作列之前） -->
      <AdminColumn
        field="createdDateLabel"
        :header="USERS_MESSAGES.joinedAtColumn"
        :style="{ width: '130px' }"
      >
        <template #body="{ data: user }">
          <AdminMono class="whitespace-nowrap text-xs text-foreground tabular-nums">
            {{ user.createdDateLabel }}
          </AdminMono>
        </template>
      </AdminColumn>

      <!-- 更多操作列 (1:1 对齐事件表第 10 列 More) -->
      <AdminColumn header="More" :style="{ width: ADMIN_TABLE_MORE_COLUMN_WIDTH }">
        <template #body="{ data: user }">
          <div class="flex items-center justify-start">
            <button
              type="button"
              :class="adminTableRowActionButtonClass"
              aria-haspopup="dialog"
              :aria-label="USERS_MESSAGES.actionsMenuAria(user.displayName)"
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
</template>

<script setup lang="ts">
import { Discord, Globe, Google, Mail, MoreHoriz, Refresh } from '@iconoir/vue'
import { computed, ref } from 'vue'
import { AdminStatusPill } from '../../../shared'
import { AdminButton } from '../../../shared'
import { AdminCollectionPageShell } from '../../../shared'
import { AdminColumn } from '../../../shared'
import { AdminEmptyState } from '../../../shared'
import { AdminFilterPopover } from '../../../shared'
import { AdminMono } from '../../../shared'
import { AdminRowActionsPopover } from '../../../shared'
import { AdminSelect } from '../../../shared'
import { AdminTable } from '../../../shared'
import { AdminToolbarSearch } from '../../../shared'
import { useAdminConfirm } from '../../../shared'
import {
  adminFramedIconButtonClass,
  adminTableClass,
  adminTableFrameClass,
  adminTableRowActionButtonClass,
  adminTableStyle,
} from '../../../shared'
import { adminInlineJumpClass } from '../../../shared'
import { emptyPlaceholder } from '../../../shared'
import { ADMIN_TABLE_MORE_COLUMN_WIDTH } from '../../../shared'
import AdminUserWalletDialog from '../../../credits/ui/wallets/AdminUserWalletDialog.vue'
import AdminUserBanDialog from '../dialog/AdminUserBanDialog.vue'
import { banDurationOptions } from '../../session/commands'
import { useAdminUsersContext } from '../../session/context'
import { useUsersFeedback } from '../../session/useUsersFeedback'
import { USERS_MESSAGES } from './messages'
import {
  getCountryFlag,
  getCountryLabel,
  type AdminUserRowView,
  type UserActionKey,
} from '../../core/view'

type ActionPopupPanel = {
  toggle: (event: Event) => void
  hide?: () => void
}

const feedback = useUsersFeedback()
const adminConfirm = useAdminConfirm()
const userActionPopoverRef = ref<ActionPopupPanel | null>(null)
const walletDialogUser = ref<AdminUserRowView | null>(null)
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
