<template>
  <AdminTablePanel viewport-height content-class="min-h-0 flex-1 overflow-hidden px-6 pb-6">
    <template #toolbar>
      <AdminResourceToolbar>
        <template #metrics="{ metricClass }">
          <span :class="metricClass">当前页 {{ rewardCodes.length }} 个兑换码</span>
        </template>
        <template #actions="{ iconButtonClass }">
          <Button text :class="iconButtonClass" aria-label="刷新兑换码" @click="loadRewardCodes">
            <template #icon><Refresh class="size-4 text-admin-text-2" /></template>
          </Button>
          <Button
            label="创建兑换码"
            size="small"
            :disabled="creating"
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

    <Message v-if="loadError" severity="error" :closable="false" class="m-5">
      <div class="flex flex-wrap items-center justify-between gap-3">
        <span>兑换码加载失败，请重试。</span>
        <Button label="重试" severity="secondary" size="small" outlined @click="loadRewardCodes" />
      </div>
    </Message>
    <DataTableFrame
      v-else
      :value="rewardCodes"
      :loading="loading"
      data-key="id"
      scrollable
      scroll-height="flex"
      frame-class="h-full min-h-0"
      table-aria-label="兑换码列表"
      scroll-aria-label="兑换码列表横向滚动区域"
      empty-title="暂无兑换码"
      empty-description="创建第一个兑换码后，用户即可在奖励中心兑换 Idea。"
      :table-style="{ minWidth: '860px' }"
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
      <Column field="status" header="状态" :style="{ width: '112px' }">
        <template #body="{ data }">
          <Tag
            :value="statusPresentation(data.status).label"
            :severity="statusPresentation(data.status).severity"
            rounded
          />
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
    </DataTableFrame>
  </AdminTablePanel>

  <RewardCodeCreateDialog
    v-model:visible="createDialogVisible"
    :pending="creating"
    :error-message="createError"
    @submit="createRewardCode"
  />
  <RewardCodePlaintextDialog
    :visible="plaintextDialogVisible"
    :plaintext="createdPlaintext"
    @update:visible="handlePlaintextDialogVisible"
  />
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { Refresh } from '@iconoir/vue'
import Button from 'primevue/button'
import Column from 'primevue/column'
import Message from 'primevue/message'
import Tag from 'primevue/tag'
import DataTableFrame from '../../../../shared/components/DataTableFrame.vue'
import { formatIdeaAmount } from '../../../../shared/utils/ideaCurrency'
import AdminPagerControls from '../../shared/components/AdminPagerControls.vue'
import AdminResourceToolbar from '../../shared/components/AdminResourceToolbar.vue'
import AdminTablePanel from '../../shared/components/AdminTablePanel.vue'
import { useAdminCursorPagination } from '../../shared/composables/useAdminCursorPagination'
import { formatAdminDisplayError, useAdminToast } from '../../shared/lib/adminToast'
import RewardCodeCreateDialog from './RewardCodeCreateDialog.vue'
import RewardCodePlaintextDialog from './RewardCodePlaintextDialog.vue'
import {
  createAdminRewardCodeResource,
  type CreateRewardCodeInput,
  type RewardCode,
} from './rewardCodeAdminResource'

const resource = createAdminRewardCodeResource()
const toast = useAdminToast()
const rewardCodes = ref<RewardCode[]>([])
const loading = ref(true)
const loadError = ref(false)
const creating = ref(false)
const createError = ref('')
const createDialogVisible = ref(false)
const plaintextDialogVisible = ref(false)
const createdPlaintext = ref('')
const pagination = useAdminCursorPagination<number>({ pending: loading })

onMounted(loadRewardCodes)

async function loadRewardCodes() {
  loading.value = true
  loadError.value = false
  try {
    const page = await resource.list(pagination.cursor.value)
    rewardCodes.value = page.rewardCodes
    pagination.setNextCursor(page.nextBeforeId)
  } catch (error) {
    loadError.value = true
    toast.error(formatAdminDisplayError(error, '兑换码加载失败'))
  } finally {
    loading.value = false
  }
}

function openCreateDialog() {
  if (creating.value) return
  createError.value = ''
  createDialogVisible.value = true
}

async function createRewardCode(input: CreateRewardCodeInput) {
  creating.value = true
  createError.value = ''
  try {
    const created = await resource.create(input)
    createDialogVisible.value = false
    createdPlaintext.value = created.plaintext || input.code
    plaintextDialogVisible.value = true
    toast.success('兑换码已创建')
    pagination.reset()
    void loadRewardCodes()
  } catch (error) {
    createError.value = formatAdminDisplayError(error, '创建兑换码失败')
  } finally {
    creating.value = false
  }
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
