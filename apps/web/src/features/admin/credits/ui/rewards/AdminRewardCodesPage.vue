<template>
  <AdminCollectionPageShell
    :current-page="pagination.currentPage.value"
    :total-pages="pagination.knownPageCount.value"
    :prev-disabled="!pagination.canRetreat.value"
    :next-disabled="!pagination.canAdvance.value"
    @prev="previousPage"
    @next="nextPage"
  >
    <template #toolbar-start>
      <AdminToolbarSearch
        v-model="searchKeyword"
        :input-label="REWARD_CODES_MESSAGES.searchLabel"
        :placeholder="REWARD_CODES_MESSAGES.searchPlaceholder"
        class="w-full sm:w-80"
        @enter="searchRewardCodes"
        @clear="clearSearch"
      />
      <ActionButton :disabled="saving" @click="openCreateDialog">
        {{ REWARD_CODES_MESSAGES.create }}
      </ActionButton>
    </template>

    <template #toolbar-end>
      <AdminFilterPopover
        :active="statusFilter !== REWARD_CODE_STATUS_FILTER_ALL"
        :filter-label="REWARD_CODES_MESSAGES.filterLabel"
        @reset="resetStatusFilter"
      >
        <label
          class="flex min-w-0 flex-col gap-1.5 text-admin-meta font-admin-strong text-muted-foreground"
        >
          <span>{{ REWARD_CODES_MESSAGES.activeStateLabel }}</span>
          <AdminSelect v-model="statusFilter" :options="statusOptions" size="small" />
        </label>
      </AdminFilterPopover>

      <AdminButton
        variant="ghost"
        :class="adminFramedIconButtonClass"
        :aria-label="REWARD_CODES_MESSAGES.refresh"
        :loading="loading"
        :disabled="loading"
        @click="refreshRewardCodes"
      >
        <Refresh class="size-4 text-muted-foreground" />
      </AdminButton>
    </template>

    <template #alerts>
      <AdminAlert v-if="backgroundError" severity="warn" class="mx-4 mt-4">
        <div class="flex flex-wrap items-center justify-between gap-3">
          <span>{{ REWARD_CODES_MESSAGES.backgroundError }}</span>
          <ActionButton variant="outline" :disabled="loading" @click="refreshRewardCodes">
            {{ REWARD_CODES_MESSAGES.retry }}
          </ActionButton>
        </div>
      </AdminAlert>
    </template>

    <AdminTable
      v-if="!initialError"
      :value="displayRewardCodes"
      :loading="loading"
      data-key="id"
      scrollable
      :frame-class="adminTableFrameClass"
      :table-aria-label="REWARD_CODES_MESSAGES.tableAria"
      :scroll-aria-label="REWARD_CODES_MESSAGES.tableScrollAria"
      :table-style="adminTableStyle"
      :class="adminTableClass"
    >
      <template #empty>
        <AdminEmptyState :title="emptyTitle" />
      </template>

      <!-- 兑换码列：名称 + 掩码 Code -->
      <AdminColumn
        field="displayName"
        :header="REWARD_CODES_MESSAGES.codeColumn"
        :style="{ minWidth: '220px' }"
      >
        <template #body="{ data }">
          <div class="min-w-0">
            <div class="truncate font-medium text-foreground">
              {{ data.displayName }}
            </div>
            <div class="mt-0.5 font-mono text-xs text-muted-foreground">
              {{ data.maskedCodeHint }}
            </div>
          </div>
        </template>
      </AdminColumn>

      <!-- 奖励 Idea 列：等宽字体金额 -->
      <AdminColumn
        field="creditAmount"
        :header="REWARD_CODES_MESSAGES.ideaColumn"
        :style="{ width: '140px' }"
      >
        <template #body="{ data }">
          <span class="font-mono text-xs font-semibold text-foreground">
            {{ formatIdeaAmount(data.creditAmount) }}
          </span>
        </template>
      </AdminColumn>

      <!-- 状态列（载体统一 StatusBadge；启用只用开关，胶囊保留生命周期） -->
      <AdminColumn
        field="status"
        :header="REWARD_CODES_MESSAGES.statusColumn"
        :style="{ width: '180px' }"
      >
        <template #body="{ data }">
          <!-- 防抖行收口到 SwitchRow：68px status 槽锁宽 + switch 固定位，不绑 key。 -->
          <SwitchRow
            :pill="statusPresentation(data.status).label"
            :pill-tone="statusPresentation(data.status).tone"
            status-min-width="68px"
            :model-value="data.enabled"
            :switch-attrs="{
              disabled: enabledToggle.isPending(data.id),
              'aria-label': REWARD_CODES_MESSAGES.enabledAria(data.displayName),
            }"
            @update:model-value="setRewardCodeEnabled(data, $event)"
          />
        </template>
      </AdminColumn>

      <!-- 兑换进度列：等宽字体分子分母 -->
      <AdminColumn :header="REWARD_CODES_MESSAGES.progressColumn" :style="{ width: '150px' }">
        <template #body="{ data }">
          <span class="font-mono text-xs text-foreground">
            {{ data.redemptionCount }} / {{ data.maxRedemptions === 0 ? '∞' : data.maxRedemptions }}
          </span>
        </template>
      </AdminColumn>

      <!-- 过期时间列：等宽日期 -->
      <AdminColumn
        field="expiresAt"
        :header="REWARD_CODES_MESSAGES.expiresColumn"
        :style="{ width: '180px' }"
      >
        <template #body="{ data }">
          <span class="whitespace-nowrap font-mono text-xs text-muted-foreground">
            {{ formatExpiresAt(data.expiresAt) }}
          </span>
        </template>
      </AdminColumn>

      <!-- 更多操作列 (1:1 对齐用户管理页 More 列) -->
      <AdminColumn header="More" :style="{ width: ADMIN_TABLE_MORE_COLUMN_WIDTH }">
        <template #body="{ data }">
          <div class="flex items-center justify-start">
            <button
              type="button"
              :class="adminTableRowActionButtonClass"
              aria-haspopup="dialog"
              :aria-label="REWARD_CODES_MESSAGES.actionsMenuAria(data.displayName)"
              @click="toggleActionMenu($event, data)"
            >
              <MoreHoriz class="size-4" />
            </button>
          </div>
        </template>
      </AdminColumn>
    </AdminTable>

    <template #footer-label>{{ totalRewardCodesLabel }}</template>
  </AdminCollectionPageShell>

  <AdminRowActionsPopover
    ref="actionPopoverRef"
    :items="rewardCodeActionItems"
    @select="selectAction"
    @hide="activeRewardCode = null"
  />

  <RewardCodeFormDialog
    v-model:visible="createDialogVisible"
    :reward-code="editingRewardCode"
    :pending="saving"
    @submit="saveRewardCode"
  />
  <RewardCodePlaintextDialog
    :visible="plaintextDialogVisible"
    :plaintext="createdPlaintext"
    :header="plaintextHeader"
    @update:visible="handlePlaintextDialogVisible"
  />
  <RewardCodeRedemptionsDialog
    v-if="redemptionsRewardCode"
    v-model:visible="redemptionsDialogVisible"
    :reward-code="redemptionsRewardCode"
  />
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { MoreHoriz, Refresh } from '@iconoir/vue'
import { AdminCollectionPageShell } from '../../../shared'
import { AdminColumn } from '../../../shared'
import { AdminAlert } from '../../../shared'
import type { AppStatusTone } from '../../../shared'
import { SwitchRow } from '../../../shared'
import { AdminButton } from '../../../shared'
import ActionButton from '@/core/components/ActionButton.vue'
import { AdminSelect } from '../../../shared'
import { AdminTable } from '../../../shared'
import { formatIdeaAmount } from '@/core/i18n/userIdeaCurrency'
import { formatLocaleDate } from '@/core/i18n/formatters'
import { AdminRowActionsPopover } from '../../../shared'
import { AdminToolbarSearch } from '../../../shared'
import { AdminEmptyState } from '../../../shared'
import { AdminFilterPopover } from '../../../shared'
import { useAdminToast } from '../../../shared'
import {
  adminFramedIconButtonClass,
  adminTableClass,
  adminTableFrameClass,
  adminTableRowActionButtonClass,
  adminTableStyle,
} from '../../../shared'
import { ADMIN_TABLE_MORE_COLUMN_WIDTH } from '../../../shared'
import { reconcileStableRows } from '../../../shared'
import RewardCodeFormDialog, { type RewardCodeFormSubmit } from './RewardCodeFormDialog.vue'
import RewardCodePlaintextDialog from './RewardCodePlaintextDialog.vue'
import RewardCodeRedemptionsDialog from './RewardCodeRedemptionsDialog.vue'
import {
  createAdminRewardCodeResource,
  REWARD_CODE_STATUS_FILTER_ALL,
  type RewardCode,
} from '../../session/rewards/resource'
import { useRewardCodeList } from '../../session/rewards/useRewardCodeList'
import { useRewardCodeRowActions } from '../../session/rewards/useRewardCodeRowActions'
import { useRewardCodesFeedback } from '../../session/rewards/useRewardCodesFeedback'
import { useConfirm } from '@/core/composables/useConfirmDialog'
import { confirmSettingsSave } from '../../../shared'
import { REWARD_CODES_MESSAGES } from './messages'

const resource = createAdminRewardCodeResource()
const confirm = useConfirm()
const {
  searchKeyword,
  statusFilter,
  statusOptions,
  rewardCodes,
  totalRewardCodesLabel,
  emptyTitle,
  loading,
  initialError,
  backgroundError,
  pagination,
  previousPage,
  nextPage,
  rewardCodeListQuery,
  updateRewardCodes,
  refreshRewardCodes,
  searchRewardCodes,
  clearSearch,
  resetStatusFilter,
} = useRewardCodeList({ resource })
const {
  toggle: enabledToggle,
  deletePending,
  replaceRewardCode,
  setRewardCodeEnabled,
  confirmDelete,
} = useRewardCodeRowActions({ resource, updateRewardCodes, refreshRewardCodes })

// 行开关乐观投影 + 引用稳定：enabled 走共享 toggle.effective 即时翻转，
// status 胶囊四态仍取服务端 status（投影不碰 status）；内容一致复用旧引用，
// data-key="id" 保证行 DOM 复用不 remount。
const rewardRowCache = new Map<number, RewardCode>()

function isSameRewardRow(cached: RewardCode, incoming: RewardCode): boolean {
  return (
    cached.id === incoming.id &&
    cached.displayName === incoming.displayName &&
    cached.maskedCodeHint === incoming.maskedCodeHint &&
    cached.creditAmount === incoming.creditAmount &&
    cached.maxRedemptions === incoming.maxRedemptions &&
    cached.redemptionCount === incoming.redemptionCount &&
    cached.expiresAt === incoming.expiresAt &&
    cached.enabled === incoming.enabled &&
    cached.status === incoming.status &&
    cached.createdAt === incoming.createdAt &&
    cached.updatedAt === incoming.updatedAt
  )
}

const displayRewardCodes = computed(() => {
  const projected = rewardCodes.value.map((row) => ({
    ...row,
    enabled: enabledToggle.effective(row.id, row.enabled),
  }))
  return reconcileStableRows(rewardRowCache, projected, (row) => row.id, isSameRewardRow)
})

// 服务端追上乐观值后清掉覆盖，避免长期遮挡外部变更；不等则保留。
watch(
  rewardCodes,
  (rows) => {
    enabledToggle.reconcileAll(rows.map((row) => ({ key: row.id, enabled: row.enabled })))
  },
  { deep: true },
)
const toast = useAdminToast()
const codesFeedback = useRewardCodesFeedback()
const saving = ref(false)
const createDialogVisible = ref(false)
const editingRewardCode = ref<RewardCode | null>(null)
const plaintextDialogVisible = ref(false)
const createdPlaintext = ref('')
const plaintextHeader = ref<string>(REWARD_CODES_MESSAGES.plaintextHeader)
const activeRewardCode = ref<RewardCode | null>(null)
const redemptionsRewardCode = ref<RewardCode | null>(null)
const redemptionsDialogVisible = ref(false)
const actionPopoverRef = ref<ActionPopupPanel | null>(null)
const rewardCodeActionItems = computed(() => [
  { key: 'redemptions', label: REWARD_CODES_MESSAGES.redemptionsAction },
  { key: 'edit', label: REWARD_CODES_MESSAGES.editAction },
  {
    key: 'delete',
    label: REWARD_CODES_MESSAGES.deleteAction,
    danger: true,
    disabled: Boolean(deletePending[activeRewardCode.value?.id ?? 0]),
  },
])
type ActionPopupPanel = { toggle: (event: Event) => void; hide?: () => void }

function openCreateDialog() {
  if (saving.value) return
  editingRewardCode.value = null
  createDialogVisible.value = true
}

async function confirmRewardCodeCreate(displayName: string) {
  return confirmSettingsSave(confirm, {
    title: REWARD_CODES_MESSAGES.confirmCreateTitle,
    message: REWARD_CODES_MESSAGES.confirmCreateMessage(displayName),
    confirmLabel: REWARD_CODES_MESSAGES.confirmCreate,
  })
}

async function persistRewardCode(input: RewardCodeFormSubmit, editing: RewardCode | null) {
  if (!editing) {
    const created = await resource.create(input)
    createdPlaintext.value = created.plaintext || input.code
    plaintextHeader.value = REWARD_CODES_MESSAGES.plaintextHeader
    plaintextDialogVisible.value = true
    toast.success(REWARD_CODES_MESSAGES.created)
  } else {
    const updated = await resource.update(editing.id, {
      displayName: input.displayName,
      creditAmount: input.creditAmount,
      maxRedemptions: input.maxRedemptions,
      expiresAt: input.expiresAt,
      ...(input.code ? { code: input.code } : {}),
    })
    replaceRewardCode(updated.rewardCode)
    if (input.code) {
      createdPlaintext.value = updated.plaintext || input.code
      plaintextHeader.value = REWARD_CODES_MESSAGES.newCodeHeader
      plaintextDialogVisible.value = true
    }
    toast.success(
      input.code ? REWARD_CODES_MESSAGES.updatedWithCode : REWARD_CODES_MESSAGES.updated,
    )
  }
}

async function saveRewardCode(input: RewardCodeFormSubmit) {
  if (saving.value) return
  const editing = editingRewardCode.value
  if (!editing) {
    const isConfirmed = await confirmRewardCodeCreate(input.displayName)
    if (!isConfirmed) return
  }
  saving.value = true
  try {
    await persistRewardCode(input, editing)
    createDialogVisible.value = false
    // Resetting the cursor reloads the first page via the key — unless
    // already there, in which case the key is unchanged and needs a refetch.
    const wasFirstPage = pagination.currentPage.value === 1
    pagination.reset()
    if (wasFirstPage) void rewardCodeListQuery.refetch()
  } catch (error) {
    codesFeedback.saveFailed(error, Boolean(editingRewardCode.value))
  } finally {
    saving.value = false
  }
}

function toggleActionMenu(event: Event, rewardCode: RewardCode) {
  activeRewardCode.value = rewardCode
  actionPopoverRef.value?.toggle(event)
}

function selectAction(action: string) {
  const rewardCode = activeRewardCode.value
  actionPopoverRef.value?.hide?.()
  if (!rewardCode) return
  if (action === 'redemptions') {
    redemptionsRewardCode.value = rewardCode
    redemptionsDialogVisible.value = true
    return
  }
  if (action === 'edit') {
    editingRewardCode.value = rewardCode
    createDialogVisible.value = true
    return
  }
  if (action === 'delete') void confirmDelete(rewardCode)
}

function handlePlaintextDialogVisible(visible: boolean) {
  plaintextDialogVisible.value = visible
  if (!visible) createdPlaintext.value = ''
}

function formatExpiresAt(value?: string): string {
  if (!value) return REWARD_CODES_MESSAGES.neverExpires
  return formatLocaleDate(new Date(value), {
    timeZone: 'Asia/Shanghai',
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  })
}

function statusPresentation(status: RewardCode['status']): {
  label: string
  tone: AppStatusTone
} {
  switch (status) {
    case 'expired':
      return { label: REWARD_CODES_MESSAGES.statusExpired, tone: 'neutral' }
    case 'exhausted':
      return { label: REWARD_CODES_MESSAGES.statusExhausted, tone: 'warning' }
    case 'disabled':
      return { label: REWARD_CODES_MESSAGES.statusDisabled, tone: 'neutral' }
    case 'enabled':
      return { label: REWARD_CODES_MESSAGES.statusEnabled, tone: 'success' }
  }
}
</script>
