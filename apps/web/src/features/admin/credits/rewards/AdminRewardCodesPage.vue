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
          class="flex min-w-0 flex-col gap-1.5 text-admin-meta font-[650] text-muted-foreground"
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
      <AdminAlert v-if="initialError" severity="error" class="m-4">
        <div class="flex flex-wrap items-center justify-between gap-3">
          <span>{{ REWARD_CODES_MESSAGES.initialError }}</span>
          <ActionButton variant="outline" :disabled="loading" @click="refreshRewardCodes">
            {{ REWARD_CODES_MESSAGES.retry }}
          </ActionButton>
        </div>
      </AdminAlert>
      <AdminAlert v-else-if="backgroundError" severity="warn" class="mx-4 mt-4">
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
      :value="rewardCodes"
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

      <!-- 状态列：Badge + 开关 -->
      <AdminColumn
        field="status"
        :header="REWARD_CODES_MESSAGES.statusColumn"
        :style="{ width: '180px' }"
      >
        <template #body="{ data }">
          <div class="flex items-center gap-3">
            <AdminBadge
              :value="statusPresentation(data.status).label"
              :severity="statusPresentation(data.status).severity"
              rounded
            />
            <AdminSwitch
              :key="`${data.id}:${data.enabled}:${Boolean(enabledPending[data.id])}`"
              :model-value="data.enabled"
              :disabled="Boolean(enabledPending[data.id])"
              :aria-label="REWARD_CODES_MESSAGES.enabledAria(data.displayName)"
              @update:model-value="setRewardCodeEnabled(data, $event)"
            />
          </div>
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
    :error-message="saveError"
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
import { computed, ref } from 'vue'
import { MoreHoriz, Refresh } from '@iconoir/vue'
import AdminCollectionPageShell from '../../shared/components/AdminCollectionPageShell.vue'
import AdminColumn from '../../shared/components/AdminColumn.vue'
import AdminAlert from '../../shared/components/AdminAlert.vue'
import AdminBadge from '../../shared/components/AdminBadge.vue'
import AdminButton from '../../shared/components/AdminButton.vue'
import ActionButton from '../../../../shared/components/ActionButton.vue'
import AdminSelect from '../../shared/components/AdminSelect.vue'
import AdminSwitch from '../../shared/components/AdminSwitch.vue'
import AdminTable from '../../shared/components/AdminTable.vue'
import { formatIdeaAmount } from '../../../../shared/i18n/userIdeaCurrency'
import { formatLocaleDate } from '../../../../shared/i18n/formatters'
import AdminRowActionsPopover from '../../shared/components/AdminRowActionsPopover.vue'
import AdminToolbarSearch from '../../shared/components/AdminToolbarSearch.vue'
import AdminEmptyState from '../../shared/components/AdminEmptyState.vue'
import AdminFilterPopover from '../../shared/components/AdminFilterPopover.vue'
import { formatAdminDisplayError, useAdminToast } from '../../shared/lib/toast'
import {
  adminFramedIconButtonClass,
  adminTableClass,
  adminTableFrameClass,
  adminTableRowActionButtonClass,
  adminTableStyle,
} from '../../shared/lib/uiClasses'
import { ADMIN_TABLE_MORE_COLUMN_WIDTH } from '../../shared/lib/tableColumns'
import RewardCodeFormDialog, { type RewardCodeFormSubmit } from './RewardCodeFormDialog.vue'
import RewardCodePlaintextDialog from './RewardCodePlaintextDialog.vue'
import RewardCodeRedemptionsDialog from './RewardCodeRedemptionsDialog.vue'
import {
  createAdminRewardCodeResource,
  REWARD_CODE_STATUS_FILTER_ALL,
  type RewardCode,
} from './resource'
import { useRewardCodeList } from './useRewardCodeList'
import { useRewardCodeRowActions } from './useRewardCodeRowActions'
import { useAdminConfirm } from '../../shared/composables/useAdminConfirm'
import { confirmSettingsSave } from '../../shared/lib/confirmSettingsSave'
import { REWARD_CODES_MESSAGES } from './messages'

const resource = createAdminRewardCodeResource()
const confirm = useAdminConfirm()
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
const { enabledPending, deletePending, replaceRewardCode, setRewardCodeEnabled, confirmDelete } =
  useRewardCodeRowActions({ resource, updateRewardCodes, refreshRewardCodes })
const toast = useAdminToast()
const saving = ref(false)
const saveError = ref('')
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
  saveError.value = ''
  createDialogVisible.value = true
}

async function confirmRewardCodeSave(editing: RewardCode | null, displayName: string) {
  // Reward code create/update is reversible config: the note is optional
  // and an empty field falls back to a default so one click proceeds.
  // Returns null only when the operator cancels.
  return confirmSettingsSave(confirm, {
    title: editing
      ? REWARD_CODES_MESSAGES.confirmUpdateTitle
      : REWARD_CODES_MESSAGES.confirmCreateTitle,
    message: editing
      ? REWARD_CODES_MESSAGES.confirmUpdateMessage(editing.displayName)
      : REWARD_CODES_MESSAGES.confirmCreateMessage(displayName),
    confirmLabel: editing
      ? REWARD_CODES_MESSAGES.confirmUpdate
      : REWARD_CODES_MESSAGES.confirmCreate,
  })
}

async function persistRewardCode(
  input: RewardCodeFormSubmit,
  editing: RewardCode | null,
  operatorReason: string,
) {
  if (!editing) {
    const created = await resource.create(input, { operatorReason })
    createdPlaintext.value = created.plaintext || input.code
    plaintextHeader.value = REWARD_CODES_MESSAGES.plaintextHeader
    plaintextDialogVisible.value = true
    toast.success(REWARD_CODES_MESSAGES.created)
  } else {
    const updated = await resource.update(
      editing.id,
      {
        displayName: input.displayName,
        creditAmount: input.creditAmount,
        maxRedemptions: input.maxRedemptions,
        expiresAt: input.expiresAt,
        ...(input.code ? { code: input.code } : {}),
      },
      { operatorReason },
    )
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
  const operatorReason = await confirmRewardCodeSave(editing, input.displayName)
  if (!operatorReason) return
  saving.value = true
  saveError.value = ''
  try {
    await persistRewardCode(input, editing, operatorReason)
    createDialogVisible.value = false
    // Resetting the cursor reloads the first page via the key — unless
    // already there, in which case the key is unchanged and needs a refetch.
    const wasFirstPage = pagination.currentPage.value === 1
    pagination.reset()
    if (wasFirstPage) void rewardCodeListQuery.refetch()
  } catch (error) {
    saveError.value = formatAdminDisplayError(
      error,
      editingRewardCode.value
        ? REWARD_CODES_MESSAGES.updateFailed
        : REWARD_CODES_MESSAGES.createFailed,
    )
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
    saveError.value = ''
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

function statusPresentation(status: RewardCode['status']) {
  switch (status) {
    case 'expired':
      return { label: REWARD_CODES_MESSAGES.statusExpired, severity: 'secondary' as const }
    case 'exhausted':
      return { label: REWARD_CODES_MESSAGES.statusExhausted, severity: 'warn' as const }
    case 'disabled':
      return { label: REWARD_CODES_MESSAGES.statusDisabled, severity: 'secondary' as const }
    case 'enabled':
      return { label: REWARD_CODES_MESSAGES.statusEnabled, severity: 'success' as const }
  }
}
</script>
