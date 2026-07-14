<template>
  <AdminTablePanel viewport-height content-class="min-h-0 flex-1 overflow-hidden px-6 pb-6">
    <template #toolbar>
      <AdminResourceToolbar>
        <template #search>
          <AdminToolbarSearch v-model="searchKeyword" @enter="loadUsers" @clear="clearSearch" />
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
      data-key="id"
      lazy
      scrollable
      scroll-height="flex"
      frame-class="h-full min-h-0"
      :table-style="{ minWidth: '920px' }"
    >
      <Column selection-mode="multiple" style="width: 3rem" />

      <Column field="username" header="用户" :style="{ minWidth: '260px' }">
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
          <Tag :value="user.roleLabel" :severity="user.roleSeverity" rounded />
        </template>
      </Column>

      <Column header="认证 ID" :style="{ minWidth: '180px' }">
        <template #body="{ data: user }">
          <span
            class="block max-w-[168px] truncate font-mono text-admin-caption text-admin-text-3"
            :title="user.authUserIdLabel"
          >
            {{ user.authUserIdLabel }}
          </span>
        </template>
      </Column>

      <Column field="createdDateLabel" header="加入时间" :style="{ width: '136px' }">
        <template #body="{ data: user }">
          <span class="text-admin-body text-admin-text-3">
            {{ user.createdDateLabel }}
          </span>
        </template>
      </Column>

      <Column field="status" header="状态" :style="{ width: '112px' }">
        <template #body="{ data: user }">
          <Tag :value="user.statusLabel" :severity="user.statusSeverity" rounded />
        </template>
      </Column>

      <Column header="更多" :style="{ width: '72px' }">
        <template #body="{ data: user }">
          <Button
            text
            rounded
            size="small"
            severity="secondary"
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
    </DataTableFrame>
  </AdminTablePanel>

  <Popover ref="userActionPopoverRef" :pt="userActionPopoverPt" @hide="handleUserActionPopoverHide">
    <div :class="userActionPopoverClass">
      <template v-for="item in userActionItems" :key="item.key">
        <div v-if="item.divided" class="my-1 h-px bg-admin-border-soft" aria-hidden="true" />
        <Button
          :label="item.label"
          text
          :severity="item.key === 'delete' ? 'danger' : 'secondary'"
          :disabled="item.disabled"
          :class="[
            adminTextButtonClass,
            '!w-full !justify-start !rounded-md !px-3 !py-2.5 !text-admin-meta !font-[650]',
          ]"
          @click="handleActionSelect(item.key)"
        />
      </template>
    </div>
  </Popover>

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
import { Refresh, Search } from '@iconoir/vue'
import { ref } from 'vue'
import Avatar from 'primevue/avatar'
import Button from 'primevue/button'
import Column from 'primevue/column'
import Popover from 'primevue/popover'
import Select from 'primevue/select'
import Tag from 'primevue/tag'
import AdminFilterPopover from '../shared/components/AdminFilterPopover.vue'
import AdminPagerControls from '../shared/components/AdminPagerControls.vue'
import AdminResourceToolbar from '../shared/components/AdminResourceToolbar.vue'
import AdminTablePanel from '../shared/components/AdminTablePanel.vue'
import AdminToolbarSearch from '../shared/components/AdminToolbarSearch.vue'
import { useAdminConfirm } from '../shared/composables/useAdminConfirm'
import DataTableFrame from '../../../shared/components/DataTableFrame.vue'
import { createAdminPopoverPt } from '../shared/lib/adminOverlayPt'
import { adminTextButtonClass } from '../shared/lib/adminUiClasses'
import AdminUserBanDialog from './AdminUserBanDialog.vue'
import { banDurationOptions } from './adminUserCommands'
import { useAdminUsersContext } from './adminUsersContext'
import type { AdminUserRowView, UserActionKey } from './adminUserView'
import { useAdminUsersFeedback } from './useAdminUsersFeedback'

const userActionPopoverClass = 'flex min-w-[176px] flex-col gap-1'
const userActionPopoverPt = createAdminPopoverPt({ contentClass: '!p-1.5' })

type PrimePopupPanel = {
  toggle: (event: Event) => void
  hide?: () => void
}

const feedback = useAdminUsersFeedback()
const adminConfirm = useAdminConfirm()
const userActionPopoverRef = ref<PrimePopupPanel | null>(null)
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

function toggleUserActionMenu(event: Event, user: AdminUserRowView) {
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
</script>
