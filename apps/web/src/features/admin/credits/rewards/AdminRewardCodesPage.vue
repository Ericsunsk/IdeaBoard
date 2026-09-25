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
        input-label="搜索兑换码"
        placeholder="搜索名称或完整 Code"
        class="w-full sm:w-80"
        @enter="searchRewardCodes"
        @clear="clearSearch"
      />
      <AdminButton :class="adminActionButtonClass" :disabled="saving" @click="openCreateDialog">
        创建兑换码
      </AdminButton>
    </template>

    <template #toolbar-end>
      <AdminFilterPopover
        :active="statusFilter !== REWARD_CODE_STATUS_FILTER_ALL"
        filter-label="筛选兑换码状态"
        @reset="resetStatusFilter"
      >
        <label class="flex min-w-0 flex-col gap-1.5 text-admin-meta font-[650] text-admin-text-2">
          <span>有效状态</span>
          <AdminSelect v-model="statusFilter" :options="statusOptions" size="small" />
        </label>
      </AdminFilterPopover>

      <AdminButton
        variant="ghost"
        :class="adminFramedIconButtonClass"
        aria-label="刷新兑换码"
        :loading="loading"
        :disabled="loading"
        @click="refreshRewardCodes"
      >
        <Refresh class="size-4 text-admin-text-2" />
      </AdminButton>
    </template>

    <template #alerts>
      <AdminAlert v-if="initialError" severity="error" class="m-4">
        <div class="flex flex-wrap items-center justify-between gap-3">
          <span>兑换码加载失败，请重试。</span>
          <AdminButton
            variant="outline"
            size="small"
            :class="adminActionButtonClass"
            :disabled="loading"
            @click="refreshRewardCodes"
          >
            重试
          </AdminButton>
        </div>
      </AdminAlert>
      <AdminAlert v-else-if="backgroundError" severity="warn" class="mx-4 mt-4">
        <div class="flex flex-wrap items-center justify-between gap-3">
          <span>刷新兑换码失败，正在显示上次成功的数据。</span>
          <AdminButton
            variant="outline"
            size="small"
            :class="adminActionButtonClass"
            :disabled="loading"
            @click="refreshRewardCodes"
          >
            重试
          </AdminButton>
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
      table-aria-label="兑换码列表"
      scroll-aria-label="兑换码列表横向滚动区域"
      :table-style="adminTableStyle"
      :class="adminTableClass"
    >
      <template #empty>
        <AdminEmptyState :title="emptyTitle" />
      </template>

      <!-- 兑换码列：名称 + 掩码 Code -->
      <AdminColumn field="displayName" header="兑换码" :style="{ minWidth: '220px' }">
        <template #body="{ data }">
          <div class="min-w-0">
            <div class="truncate font-medium text-admin-text-1">
              {{ data.displayName }}
            </div>
            <div class="mt-0.5 font-mono text-xs text-admin-text-3">
              {{ data.maskedCodeHint }}
            </div>
          </div>
        </template>
      </AdminColumn>

      <!-- 奖励 Idea 列：等宽字体金额 -->
      <AdminColumn field="creditAmount" header="奖励 Idea" :style="{ width: '140px' }">
        <template #body="{ data }">
          <span class="font-mono text-xs font-semibold text-admin-text-1">
            {{ formatIdeaAmount(data.creditAmount) }}
          </span>
        </template>
      </AdminColumn>

      <!-- 状态列：Badge + 开关 -->
      <AdminColumn field="status" header="状态" :style="{ width: '180px' }">
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
              :aria-label="`${data.displayName}启用状态`"
              @update:model-value="setRewardCodeEnabled(data, $event)"
            />
          </div>
        </template>
      </AdminColumn>

      <!-- 兑换进度列：等宽字体分子分母 -->
      <AdminColumn header="兑换进度" :style="{ width: '150px' }">
        <template #body="{ data }">
          <span class="font-mono text-xs text-admin-text-1">
            {{ data.redemptionCount }} / {{ data.maxRedemptions === 0 ? '∞' : data.maxRedemptions }}
          </span>
        </template>
      </AdminColumn>

      <!-- 过期时间列：等宽日期 -->
      <AdminColumn field="expiresAt" header="过期时间" :style="{ width: '180px' }">
        <template #body="{ data }">
          <span class="whitespace-nowrap font-mono text-xs text-admin-text-2">
            {{ formatExpiresAt(data.expiresAt) }}
          </span>
        </template>
      </AdminColumn>

      <!-- 更多操作列 (1:1 对齐用户管理页 More 列) -->
      <AdminColumn header="More" :style="{ width: '64px' }">
        <template #body="{ data }">
          <div class="flex items-center justify-start">
            <button
              type="button"
              :class="adminTableRowActionButtonClass"
              aria-haspopup="dialog"
              :aria-label="`${data.displayName} 操作菜单`"
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
  adminActionButtonClass,
  adminFramedIconButtonClass,
  adminTableClass,
  adminTableFrameClass,
  adminTableRowActionButtonClass,
  adminTableStyle,
} from '../../shared/lib/uiClasses'
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

const resource = createAdminRewardCodeResource()
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
const plaintextHeader = ref('保存兑换码')
const activeRewardCode = ref<RewardCode | null>(null)
const redemptionsRewardCode = ref<RewardCode | null>(null)
const redemptionsDialogVisible = ref(false)
const actionPopoverRef = ref<ActionPopupPanel | null>(null)
const rewardCodeActionItems = computed(() => [
  { key: 'redemptions', label: '兑换记录' },
  { key: 'edit', label: '编辑' },
  {
    key: 'delete',
    label: '删除',
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

async function saveRewardCode(input: RewardCodeFormSubmit) {
  if (saving.value) return
  saving.value = true
  saveError.value = ''
  try {
    const editing = editingRewardCode.value
    if (!editing) {
      const created = await resource.create(input)
      createdPlaintext.value = created.plaintext || input.code
      plaintextHeader.value = '保存兑换码'
      plaintextDialogVisible.value = true
      toast.success('兑换码已创建')
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
        plaintextHeader.value = '保存新 Code'
        plaintextDialogVisible.value = true
      }
      toast.success(input.code ? '兑换码与 Code 已更新' : '兑换码已更新')
    }
    createDialogVisible.value = false
    // Resetting the cursor reloads the first page via the key — unless
    // already there, in which case the key is unchanged and needs a refetch.
    const wasFirstPage = pagination.currentPage.value === 1
    pagination.reset()
    if (wasFirstPage) void rewardCodeListQuery.refetch()
  } catch (error) {
    saveError.value = formatAdminDisplayError(
      error,
      editingRewardCode.value ? '更新兑换码失败' : '创建兑换码失败',
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
  if (!value) return '永不过期'
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
      return { label: '已过期', severity: 'secondary' as const }
    case 'exhausted':
      return { label: '已耗尽', severity: 'warn' as const }
    case 'disabled':
      return { label: '已停用', severity: 'secondary' as const }
    case 'enabled':
      return { label: '可兑换', severity: 'success' as const }
  }
}
</script>
