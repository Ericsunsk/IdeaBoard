<template>
  <AdminTablePanel viewport-height content-class="min-h-0 flex-1 overflow-hidden px-6 pb-6">
    <template #toolbar>
      <AdminResourceToolbar>
        <template #search>
          <AdminToolbarSearch
            v-model="searchKeyword"
            input-label="搜索兑换码"
            placeholder="搜索名称或完整 Code"
            @enter="searchRewardCodes"
            @clear="clearSearch"
          />
        </template>
        <template #metrics="{ metricClass }">
          <span :class="metricClass">
            <span class="h-2.5 w-2.5 rounded-full bg-admin-success"></span>
            <span>{{ totalRewardCodesLabel }}</span>
          </span>
        </template>
        <template #filters>
          <AdminFilterPopover
            :active="Boolean(statusFilter)"
            filter-label="筛选兑换码状态"
            @reset="resetStatusFilter"
          >
            <label
              class="flex min-w-0 flex-col gap-1.5 text-admin-meta font-[650] text-admin-text-2"
            >
              <span>有效状态</span>
              <Select
                v-model="statusFilter"
                :options="statusOptions"
                option-label="label"
                option-value="value"
                placeholder="全部状态"
                size="small"
                show-clear
                fluid
              />
            </label>
          </AdminFilterPopover>
        </template>
        <template #actions="{ iconButtonClass }">
          <Button
            text
            :class="iconButtonClass"
            :disabled="loading"
            aria-label="刷新兑换码"
            @click="refreshRewardCodes"
          >
            <template #icon><Refresh class="size-4 text-admin-text-2" /></template>
          </Button>
          <Button
            label="创建兑换码"
            :class="adminActionButtonClass"
            severity="contrast"
            :disabled="saving"
            @click="openCreateDialog"
          />
        </template>
        <template #pager>
          <AdminPagerControls
            :current-page="pagination.currentPage.value"
            :total-pages="pagination.knownPageCount.value"
            :prev-disabled="!pagination.canRetreat.value"
            :next-disabled="!pagination.canAdvance.value"
            @prev="previousPage"
            @next="nextPage"
          />
        </template>
      </AdminResourceToolbar>
    </template>

    <Message v-if="initialError" severity="error" :closable="false" class="m-5">
      <div class="flex flex-wrap items-center justify-between gap-3">
        <span>兑换码加载失败，请重试。</span>
        <Button
          label="重试"
          severity="secondary"
          size="small"
          outlined
          :class="adminActionButtonClass"
          :disabled="loading"
          @click="refreshRewardCodes"
        />
      </div>
    </Message>
    <Message v-else-if="backgroundError" severity="warn" :closable="false" class="mx-5 mt-5">
      <div class="flex flex-wrap items-center justify-between gap-3">
        <span>刷新兑换码失败，正在显示上次成功的数据。</span>
        <Button
          label="重试"
          severity="secondary"
          size="small"
          outlined
          :class="adminActionButtonClass"
          :disabled="loading"
          @click="refreshRewardCodes"
        />
      </div>
    </Message>
    <AdminEmptyState
      v-if="showEmptyState"
      :title="emptyTitle"
    />
    <DataTableFrame
      v-else-if="!initialError"
      :value="rewardCodes"
      :loading="loading"
      data-key="id"
      scrollable
      scroll-height="flex"
      frame-class="h-full min-h-0"
      table-aria-label="兑换码列表"
      scroll-aria-label="兑换码列表横向滚动区域"
      :table-style="{ minWidth: '980px' }"
    >
      <Column field="displayName" header="兑换码" :style="{ minWidth: '220px' }">
        <template #body="{ data }">
          <div class="min-w-0">
            <div class="truncate text-admin-content font-bold text-admin-text-1">
              {{ data.displayName }}
            </div>
            <div class="mt-0.5 font-mono text-admin-caption text-admin-text-3">
              {{ data.maskedCodeHint }}
            </div>
          </div>
        </template>
      </Column>
      <Column field="creditAmount" header="奖励 Idea" :style="{ width: '140px' }">
        <template #body="{ data }">{{ formatIdeaAmount(data.creditAmount) }}</template>
      </Column>
      <Column field="status" header="状态" :style="{ width: '180px' }">
        <template #body="{ data }">
          <div class="flex items-center gap-3">
            <Tag
              :value="statusPresentation(data.status).label"
              :severity="statusPresentation(data.status).severity"
              rounded
            />
            <ToggleSwitch
              :key="`${data.id}:${data.enabled}:${Boolean(enabledPending[data.id])}`"
              :model-value="data.enabled"
              :disabled="Boolean(enabledPending[data.id])"
              :aria-label="`${data.displayName}启用状态`"
              @update:model-value="setRewardCodeEnabled(data, $event)"
            />
          </div>
        </template>
      </Column>
      <Column header="兑换进度" :style="{ width: '150px' }">
        <template #body="{ data }">
          {{ data.redemptionCount }} / {{ data.maxRedemptions === 0 ? '∞' : data.maxRedemptions }}
        </template>
      </Column>
      <Column field="expiresAt" header="过期时间" :style="{ width: '180px' }">
        <template #body="{ data }">
          <span class="text-admin-body text-admin-text-3">{{
            formatExpiresAt(data.expiresAt)
          }}</span>
        </template>
      </Column>
      <Column header="更多" :style="{ width: '72px' }">
        <template #body="{ data }">
          <Button
            text
            size="small"
            severity="secondary"
            :class="adminFramedIconButtonClass"
            aria-haspopup="dialog"
            :aria-label="`${data.displayName} 操作菜单`"
            @click="toggleActionMenu($event, data)"
          >
            <template #icon><MoreHoriz class="size-4 text-admin-text-2" /></template>
          </Button>
        </template>
      </Column>
    </DataTableFrame>
  </AdminTablePanel>

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
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { MoreHoriz, Refresh } from '@iconoir/vue'
import Button from 'primevue/button'
import Column from 'primevue/column'
import Message from 'primevue/message'
import Select from 'primevue/select'
import Tag from 'primevue/tag'
import ToggleSwitch from 'primevue/toggleswitch'
import DataTableFrame from '../../../../shared/components/DataTableFrame.vue'
import { formatIdeaAmount } from '../../../../shared/utils/ideaCurrency'
import AdminPagerControls from '../../shared/components/AdminPagerControls.vue'
import AdminRowActionsPopover from '../../shared/components/AdminRowActionsPopover.vue'
import AdminResourceToolbar from '../../shared/components/AdminResourceToolbar.vue'
import AdminTablePanel from '../../shared/components/AdminTablePanel.vue'
import AdminToolbarSearch from '../../shared/components/AdminToolbarSearch.vue'
import AdminEmptyState from '../../shared/components/AdminEmptyState.vue'
import AdminFilterPopover from '../../shared/components/AdminFilterPopover.vue'
import { useAdminCollectionState } from '../../shared/composables/useAdminCollectionState'
import { useAdminCursorPagination } from '../../shared/composables/useAdminCursorPagination'
import { useAdminConfirm } from '../../shared/composables/useAdminConfirm'
import { formatAdminDisplayError, useAdminToast } from '../../shared/lib/adminToast'
import {
  adminActionButtonClass,
  adminFramedIconButtonClass,
} from '../../shared/lib/adminUiClasses'
import RewardCodeFormDialog, { type RewardCodeFormValue } from './RewardCodeFormDialog.vue'
import RewardCodePlaintextDialog from './RewardCodePlaintextDialog.vue'
import RewardCodeRedemptionsDialog from './RewardCodeRedemptionsDialog.vue'
import {
  createAdminRewardCodeResource,
  type RewardCode,
  type RewardCodeStatus,
} from './rewardCodeAdminResource'

const resource = createAdminRewardCodeResource()
const toast = useAdminToast()
const searchKeyword = ref('')
const appliedSearchKeyword = ref('')
const statusFilter = ref<RewardCodeStatus>()
const saving = ref(false)
const saveError = ref('')
const createDialogVisible = ref(false)
const editingRewardCode = ref<RewardCode | null>(null)
const plaintextDialogVisible = ref(false)
const createdPlaintext = ref('')
const plaintextHeader = ref('保存兑换码')
const enabledPending = reactive<Record<number, boolean>>({})
const deletePending = reactive<Record<number, boolean>>({})
const activeRewardCode = ref<RewardCode | null>(null)
const redemptionsRewardCode = ref<RewardCode | null>(null)
const redemptionsDialogVisible = ref(false)
const actionPopoverRef = ref<PrimePopupPanel | null>(null)
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
const confirm = useAdminConfirm()
let pagination: ReturnType<typeof useAdminCursorPagination<number>>
const collection = useAdminCollectionState({
  load: (signal) =>
    resource.search({
      query: appliedSearchKeyword.value,
      status: statusFilter.value,
      beforeId: pagination.cursor.value,
      signal,
    }),
  isEmpty: (page) => page.rewardCodes.length === 0,
  onInitialError: (error) => toast.error(formatAdminDisplayError(error, '兑换码加载失败')),
})
const { data: rewardCodePage, loading, initialError, backgroundError, updateData } = collection
pagination = useAdminCursorPagination<number>({ pending: loading })
const rewardCodes = computed(() => rewardCodePage.value?.rewardCodes ?? [])
const totalRewardCodes = computed(() => rewardCodePage.value?.total ?? 0)
const totalRewardCodesLabel = computed(() => '共 ' + totalRewardCodes.value + ' 个兑换码')
const showEmptyState = computed(
  () => !loading.value && !initialError.value && collection.isEmpty.value,
)
const emptyTitle = computed(() =>
  appliedSearchKeyword.value.trim() || statusFilter.value ? '未找到匹配的兑换码' : '暂无兑换码',
)
const statusOptions = [
  { label: '可兑换', value: 'enabled' },
  { label: '已停用', value: 'disabled' },
  { label: '已过期', value: 'expired' },
  { label: '已耗尽', value: 'exhausted' },
] satisfies Array<{ label: string; value: RewardCodeStatus }>
type PrimePopupPanel = { toggle: (event: Event) => void; hide?: () => void }

onMounted(() => {
  void loadRewardCodes()
})

onBeforeUnmount(() => collection.cancel())

watch(statusFilter, () => {
  resetCriteriaAndLoad()
})

async function loadRewardCodes({ preserveData = false }: { preserveData?: boolean } = {}) {
  const page = await collection.reload({ preserveData })
  if (page) pagination.setNextCursor(page.nextBeforeId)
}

function refreshRewardCodes() {
  void loadRewardCodes({ preserveData: true })
}

function searchRewardCodes() {
  appliedSearchKeyword.value = searchKeyword.value
  resetCriteriaAndLoad()
}

function clearSearch() {
  appliedSearchKeyword.value = ''
  resetCriteriaAndLoad()
}

function resetStatusFilter() {
  statusFilter.value = undefined
}

function resetCriteriaAndLoad() {
  pagination.reset()
  void loadRewardCodes()
}

function openCreateDialog() {
  if (saving.value) return
  editingRewardCode.value = null
  saveError.value = ''
  createDialogVisible.value = true
}

async function saveRewardCode(input: RewardCodeFormValue) {
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
    pagination.reset()
    void loadRewardCodes()
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
  if (action === 'delete') confirmDelete(rewardCode)
}

function confirmDelete(rewardCode: RewardCode) {
  if (deletePending[rewardCode.id]) return
  confirm.request({
    title: '删除兑换码',
    message: `确定永久删除“${rewardCode.displayName}”吗？没有 Redemption 的兑换码才可删除。`,
    confirmLabel: '确认删除',
    intent: 'danger',
    onConfirm: () => deleteRewardCode(rewardCode),
  })
}

async function deleteRewardCode(rewardCode: RewardCode) {
  if (deletePending[rewardCode.id]) return
  deletePending[rewardCode.id] = true
  try {
    await resource.delete(rewardCode.id)
    updateData((page) => ({
      ...page,
      rewardCodes: page.rewardCodes.filter((candidate) => candidate.id !== rewardCode.id),
    }))
    toast.success('兑换码已删除')
    void loadRewardCodes({ preserveData: true })
  } catch (error) {
    toast.error(formatAdminDisplayError(error, '删除兑换码失败'))
  } finally {
    deletePending[rewardCode.id] = false
  }
}

async function setRewardCodeEnabled(rewardCode: RewardCode, enabled: boolean) {
  if (enabledPending[rewardCode.id]) return
  enabledPending[rewardCode.id] = true
  try {
    replaceRewardCode(await resource.setEnabled(rewardCode.id, enabled))
    toast.success(enabled ? '兑换码已启用' : '兑换码已停用')
    void loadRewardCodes({ preserveData: true })
  } catch (error) {
    toast.error(formatAdminDisplayError(error, enabled ? '启用兑换码失败' : '停用兑换码失败'))
  } finally {
    enabledPending[rewardCode.id] = false
  }
}

function replaceRewardCode(updated: RewardCode) {
  updateData((page) => ({
    ...page,
    rewardCodes: page.rewardCodes.map((candidate) =>
      candidate.id === updated.id ? updated : candidate,
    ),
  }))
}

function previousPage() {
  if (!pagination.retreat()) return
  void loadRewardCodes()
}

function nextPage() {
  if (!pagination.advance()) return
  void loadRewardCodes()
}

function handlePlaintextDialogVisible(visible: boolean) {
  plaintextDialogVisible.value = visible
  if (!visible) createdPlaintext.value = ''
}

function formatExpiresAt(value?: string): string {
  if (!value) return '永不过期'
  return new Intl.DateTimeFormat('zh-CN', {
    timeZone: 'Asia/Shanghai',
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  }).format(new Date(value))
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
