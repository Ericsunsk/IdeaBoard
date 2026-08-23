<template>
  <AdminTablePanel viewport-height content-class="min-h-0 flex-1 overflow-hidden px-6 pb-6">
    <template #toolbar>
      <AdminResourceToolbar>
        <template #search>
          <AdminToolbarSearch
            v-model="searchKeyword"
            input-label="搜索用户"
            @enter="loadUsers"
            @clear="clearSearch"
          />
        </template>

        <template #metrics="{ metricClass }">
          <span :class="metricClass">
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

        <template #filters>
          <Button
            v-tooltip.top="'OAuth Provider 设置'"
            text
            :class="adminFramedIconButtonClass"
            aria-label="OAuth Provider 设置"
            aria-haspopup="dialog"
            @click="oauthSettingsVisible = true"
          >
            <template #icon>
              <Settings class="size-4 text-admin-text-2" />
            </template>
          </Button>
          <AdminFilterPopover
            :active="hasUserFilter"
            filter-label="筛选用户"
            @reset="resetUserFilters"
          >
            <label
              class="flex min-w-0 flex-col gap-1.5 text-admin-meta font-[650] text-admin-text-2"
            >
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

            <label
              class="flex min-w-0 flex-col gap-1.5 text-admin-meta font-[650] text-admin-text-2"
            >
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
        </template>

        <template #actions="{ iconButtonClass }">
          <Button text :class="iconButtonClass" aria-label="刷新用户" @click="loadUsers">
            <template #icon>
              <Refresh class="size-4 text-admin-text-2" />
            </template>
          </Button>
        </template>

        <template #pager>
          <AdminPagerControls
            :current-page="currentPage"
            :total-pages="totalPages"
            :prev-disabled="!canPrevious"
            :next-disabled="!canNext"
            @prev="previous"
            @next="next"
          />
        </template>
      </AdminResourceToolbar>
    </template>

    <DataTableFrame
      v-model:selection="selectedUsers"
      :value="users"
      :loading="loading"
      data-key="publicId"
      lazy
      scrollable
      scroll-height="flex"
      frame-class="h-full min-h-0"
      :table-style="{ minWidth: '1300px' }"
    >
      <Column selection-mode="multiple" style="width: 3rem" />

      <Column field="displayName" header="用户" :style="{ minWidth: '260px' }">
        <template #body="{ data: user }">
          <div class="flex min-w-0 items-center gap-3">
            <div class="relative shrink-0">
              <Avatar
                v-if="user.avatarUrl"
                :image="user.avatarUrl"
                shape="circle"
                :style="{ width: '36px', height: '36px' }"
              />
              <Avatar
                v-else
                :label="user.initial"
                shape="circle"
                :style="{
                  width: '36px',
                  height: '36px',
                  backgroundColor: 'var(--color-admin-surface-muted)',
                  color: 'var(--color-admin-accent)',
                  fontWeight: 700,
                }"
              />
              <span
                class="absolute top-0 right-0 h-2.5 w-2.5 rounded-full border-2 border-admin-card-strong"
                :class="user.isActive ? 'bg-admin-success' : 'bg-admin-danger'"
              />
            </div>
            <div class="min-w-0">
              <div class="truncate text-admin-content font-bold text-admin-text-1">
                {{ user.displayName }}
              </div>
            </div>
          </div>
        </template>
      </Column>

      <Column field="email" header="Email" :style="{ minWidth: '260px' }">
        <template #body="{ data: user }">
          <div class="flex min-w-0 items-center gap-2">
            <span
              v-for="provider in user.providers"
              :key="provider"
              v-tooltip.top="getAuthProviderLabel(provider)"
              class="flex size-5 shrink-0 items-center justify-center text-admin-text-3"
              :aria-label="getAuthProviderLabel(provider)"
            >
              <Mail v-if="provider === 'email'" class="size-4" aria-hidden="true" />
              <Google v-else-if="provider === 'google'" class="size-4" aria-hidden="true" />
              <Discord v-else-if="provider === 'discord'" class="size-4" aria-hidden="true" />
              <span v-else class="text-[10px] font-bold uppercase" aria-hidden="true">
                {{ provider.slice(0, 1) }}
              </span>
            </span>
            <span class="truncate text-admin-body text-admin-text-2">{{ user.email || '-' }}</span>
          </div>
        </template>
      </Column>

      <Column field="lastLoginIP" header="IP" :style="{ minWidth: '190px' }">
        <template #body="{ data: user }">
          <div class="flex min-w-0 items-center gap-2">
            <span
              v-if="getCountryFlag(user.lastLoginCountry)"
              v-tooltip.top="getCountryLabel(user.lastLoginCountry)"
              class="shrink-0 text-base leading-none"
              role="img"
              :aria-label="getCountryLabel(user.lastLoginCountry)"
            >
              {{ getCountryFlag(user.lastLoginCountry) }}
            </span>
            <Globe
              v-else
              v-tooltip.top="getCountryLabel(user.lastLoginCountry)"
              class="size-4 shrink-0 text-admin-text-3"
              role="img"
              :aria-label="getCountryLabel(user.lastLoginCountry)"
            />
            <span
              class="whitespace-nowrap font-mono text-admin-body text-admin-text-2"
              :title="user.lastLoginIP || undefined"
            >
              {{ user.lastLoginIP || '-' }}
            </span>
          </div>
        </template>
      </Column>

      <Column field="role" header="角色" :style="{ width: '124px' }">
        <template #body="{ data: user }">
          <Tag :value="user.roleLabel" :severity="user.roleSeverity" rounded />
        </template>
      </Column>

      <Column field="createdDateLabel" header="加入时间" :style="{ width: '136px' }">
        <template #body="{ data: user }">
          <span class="whitespace-nowrap font-mono text-admin-body text-admin-text-3">
            {{ user.createdDateLabel }}
          </span>
        </template>
      </Column>

      <Column field="status" header="状态" :style="{ width: '112px' }">
        <template #body="{ data: user }">
          <Tag :value="user.statusLabel" :severity="user.statusSeverity" rounded />
        </template>
      </Column>

      <Column field="availableCredits" header="账户余额" :style="{ width: '148px' }">
        <template #body="{ data: user }">
          <Button
            text
            size="small"
            severity="secondary"
            class="!-ml-2 !justify-start !px-2 font-mono !font-[760]"
            aria-haspopup="dialog"
            :aria-label="`打开 ${user.displayName} 的账户详情`"
            @click="openWalletDialog(user)"
          >
            {{ user.availableCreditsLabel }}
          </Button>
        </template>
      </Column>

      <Column header="更多" :style="{ width: '72px' }">
        <template #body="{ data: user }">
          <Button
            text
            size="small"
            severity="secondary"
            :class="adminFramedIconButtonClass"
            aria-haspopup="dialog"
            :aria-label="`${user.displayName} 操作菜单`"
            @click="toggleUserActionMenu($event, user)"
          >
            <template #icon>
              <Search class="size-4 text-admin-text-2" />
            </template>
          </Button>
        </template>
      </Column>

      <template #empty>
        <AdminEmptyState :title="emptyUserMessage" />
      </template>
    </DataTableFrame>
  </AdminTablePanel>

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
import { Discord, Globe, Google, Mail, Refresh, Search, Settings } from '@iconoir/vue'
import { computed, ref } from 'vue'
import Avatar from 'primevue/avatar'
import Button from 'primevue/button'
import Column from 'primevue/column'
import Select from 'primevue/select'
import Tag from 'primevue/tag'
import Tooltip from 'primevue/tooltip'
import AdminEmptyState from '../shared/components/AdminEmptyState.vue'
import AdminFilterPopover from '../shared/components/AdminFilterPopover.vue'
import AdminPagerControls from '../shared/components/AdminPagerControls.vue'
import AdminRowActionsPopover from '../shared/components/AdminRowActionsPopover.vue'
import AdminResourceToolbar from '../shared/components/AdminResourceToolbar.vue'
import AdminTablePanel from '../shared/components/AdminTablePanel.vue'
import AdminToolbarSearch from '../shared/components/AdminToolbarSearch.vue'
import { useAdminConfirm } from '../shared/composables/useAdminConfirm'
import DataTableFrame from '../../../shared/components/DataTableFrame.vue'
import AdminUserBanDialog from './AdminUserBanDialog.vue'
import OAuthProviderSettingsDialog from './OAuthProviderSettingsDialog.vue'
import { AdminUserWalletDialog } from '../credits-billing/wallets'
import { banDurationOptions } from './adminUserCommands'
import { useAdminUsersContext } from './adminUsersContext'
import {
  getCountryFlag,
  getCountryLabel,
  type AdminUserRowView,
  type UserActionKey,
} from './adminUserView'
import { useAdminUsersFeedback } from './useAdminUsersFeedback'
import { adminFramedIconButtonClass } from '../shared/lib/adminUiClasses'

const vTooltip = Tooltip

type PrimePopupPanel = {
  toggle: (event: Event) => void
  hide?: () => void
}

const feedback = useAdminUsersFeedback()
const adminConfirm = useAdminConfirm()
const userActionPopoverRef = ref<PrimePopupPanel | null>(null)
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
  selectedUsers,
  hasUserFilter,
  users,
  loading,
  activeUserCount,
  bannedUserCount,
  adminUserCount,
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
