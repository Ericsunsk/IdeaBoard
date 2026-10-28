<template>
  <AdminDialogShell
    :visible="visible"
    :header="REWARD_CODES_MESSAGES.redemptionsHeader(rewardCode.displayName)"
    :description="REWARD_CODES_MESSAGES.redemptionsDescription"
    size="wide"
    content-layout="scrollable"
    @update:visible="emit('update:visible', $event)"
  >
    <AdminTablePanel>
      <template #toolbar>
        <AdminResourceToolbar>
          <template #start>
            <div class="min-w-0">
              <div class="text-admin-title-sm font-admin-display text-foreground">
                {{ REWARD_CODES_MESSAGES.redemptionsTitle }}
              </div>
            </div>
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
          <template #actions="{ iconButtonClass }">
            <AdminButton
              variant="ghost"
              size="icon"
              :class="iconButtonClass"
              :loading="loading"
              :aria-label="REWARD_CODES_MESSAGES.redemptionsRefreshAria"
              @click="refreshRedemptions"
            >
              <Refresh class="size-4 text-muted-foreground" />
            </AdminButton>
          </template>
        </AdminResourceToolbar>
      </template>

      <AdminAlert v-if="backgroundError" severity="secondary">
        <div class="flex flex-wrap items-center justify-between gap-3">
          <span>{{ REWARD_CODES_MESSAGES.redemptionsStaleNotice }}</span>
          <AdminButton
            variant="ghost"
            size="sm"
            :class="adminTextButtonClass"
            :loading="loading"
            @click="refreshRedemptions"
          >
            {{ REWARD_CODES_MESSAGES.redemptionsRefreshRetry }}
          </AdminButton>
        </div>
      </AdminAlert>

      <AdminTable
        v-if="!initialError"
        v-model:expanded-rows="expandedRows"
        :value="redemptions"
        :loading="loading"
        data-key="id"
        scrollable
        :table-aria-label="REWARD_CODES_MESSAGES.redemptionsTableAria"
        :scroll-aria-label="REWARD_CODES_MESSAGES.redemptionsScrollAria"
        :empty-title="REWARD_CODES_MESSAGES.redemptionsEmpty"
        :table-style="{ minWidth: '620px' }"
        @row-expand="expandRedemption"
        @row-collapse="collapseRedemption"
      >
        <AdminColumn expander :style="{ width: ADMIN_TABLE_EXPANDER_COLUMN_WIDTH }" />
        <AdminColumn
          field="userPublicId"
          :header="REWARD_CODES_MESSAGES.redemptionsUserColumn"
          :style="{ minWidth: '240px' }"
        >
          <template #body="{ data }">
            <span class="font-semibold text-foreground">{{ data.displayName }}</span>
            <span class="block text-admin-caption text-muted-foreground">{{
              data.userPublicId
            }}</span>
          </template>
        </AdminColumn>
        <AdminColumn
          field="creditedAmount"
          :header="REWARD_CODES_MESSAGES.redemptionsAmountColumn"
          :style="{ width: '180px' }"
        >
          <template #body="{ data }">{{ formatIdeaAmount(data.creditedAmount) }}</template>
        </AdminColumn>
        <AdminColumn
          field="redeemedAt"
          :header="REWARD_CODES_MESSAGES.redemptionsTimeColumn"
          :style="{ width: '220px' }"
        >
          <template #body="{ data }">{{
            formatAdminDateTimeWithSeconds(data.redeemedAt)
          }}</template>
        </AdminColumn>

        <template #expansion="{ data }">
          <div class="border-y border-border bg-muted px-5 py-4">
            <div
              v-if="detailLoading && selectedRedemptionId === data.id"
              class="flex min-h-24 items-center justify-center"
            >
              <AdminSpinner
                class="size-7"
                :aria-label="REWARD_CODES_MESSAGES.redemptionsDetailLoadingAria"
              />
            </div>
            <dl
              v-else-if="selectedDetail?.id === data.id"
              class="grid gap-x-6 gap-y-4 sm:grid-cols-3"
            >
              <div>
                <dt class="text-admin-caption font-bold text-muted-foreground">Redemption ID</dt>
                <dd
                  class="mt-1 mb-0 break-all font-mono text-admin-body text-foreground select-all"
                >
                  {{ selectedDetail?.id }}
                </dd>
              </div>
              <div>
                <dt class="text-admin-caption font-bold text-muted-foreground">
                  Credits Transaction ID
                </dt>
                <dd
                  class="mt-1 mb-0 break-all font-mono text-admin-body text-foreground select-all"
                >
                  {{ selectedDetail?.creditTransactionId }}
                </dd>
              </div>
              <div>
                <dt class="text-admin-caption font-bold text-muted-foreground">masked Code hint</dt>
                <dd
                  class="mt-1 mb-0 break-all font-mono text-admin-body text-foreground select-all"
                >
                  {{ selectedDetail?.maskedCodeHint }}
                </dd>
              </div>
            </dl>
          </div>
        </template>
      </AdminTable>
    </AdminTablePanel>

    <template #footer>
      <ActionButton variant="outline" @click="emit('update:visible', false)">
        {{ REWARD_CODES_MESSAGES.close }}
      </ActionButton>
    </template>
  </AdminDialogShell>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useQueryClient } from '@tanstack/vue-query'
import { useAdminCursorCollection } from '../../../shared'
import { Refresh } from '@iconoir/vue'
import { AdminColumn } from '../../../shared'
import type { AdminTableRowCollapseEvent, AdminTableRowExpandEvent } from '../../../shared'
import { AdminAlert } from '../../../shared'
import { AdminButton } from '../../../shared'
import ActionButton from '@/core/components/ActionButton.vue'
import { AdminSpinner } from '../../../shared'
import { AdminTable } from '../../../shared'
import { formatIdeaAmount } from '@/core/i18n/userIdeaCurrency'
import { AdminDialogShell } from '../../../shared'
import { useRewardCodesFeedback } from '../../session/rewards/useRewardCodesFeedback'
import { useFailureToast } from '@/core/composables/useFailureToast'
import { AdminPagerControls } from '../../../shared'
import { AdminResourceToolbar } from '../../../shared'
import { AdminTablePanel } from '../../../shared'
import { formatAdminDateTimeWithSeconds } from '../../core/shared/format'
import { adminTextButtonClass } from '../../../shared'
import { ADMIN_TABLE_EXPANDER_COLUMN_WIDTH } from '../../../shared'
import type { RewardCode } from '../../session/rewards/resource'
import { REWARD_CODES_MESSAGES } from './messages'
import {
  createRewardCodeRedemptionResource,
  type RewardCodeRedemption,
  type RewardCodeRedemptionDetail,
} from '../../session/rewards/rewardCodeRedemptionResource'

const props = defineProps<{
  visible: boolean
  rewardCode: RewardCode
}>()

const emit = defineEmits<{
  'update:visible': [visible: boolean]
}>()

const resource = createRewardCodeRedemptionResource()
const feedback = useRewardCodesFeedback()
const expandedRows = ref<Record<string, boolean>>({})
const selectedRedemptionId = ref<number | null>(null)
const selectedDetail = ref<RewardCodeRedemptionDetail | null>(null)
const detailLoading = ref(false)
let activeDetailRequest: AbortController | undefined
let detailRequestEpoch = 0
// 游标集合生命周期收敛于 useAdminCursorCollection，query 挂载前保持 disabled，
// 关闭即清缓存以便重开必转 fresh。
const queryClient = useQueryClient()
const REDEMPTION_LIST_QUERY_ROOT = ['admin', 'credits', 'redemptions'] as const
const REDEMPTION_LIST_ID_INDEX = REDEMPTION_LIST_QUERY_ROOT.length
const REDEMPTION_LIST_CURSOR_INDEX = REDEMPTION_LIST_QUERY_ROOT.length + 1
const {
  data: redemptionPage,
  loading,
  initialError,
  backgroundError,
  refresh: refreshRedemptionList,
  query: redemptionListQuery,
  pagination,
} = useAdminCursorCollection({
  buildKey: (beforeId) => [...REDEMPTION_LIST_QUERY_ROOT, props.rewardCode.id, beforeId] as const,
  queryFn: ({ queryKey, signal }) =>
    resource.list(queryKey[REDEMPTION_LIST_ID_INDEX] as number, {
      beforeId: queryKey[REDEMPTION_LIST_CURSOR_INDEX] as number | undefined,
      signal,
    }),
  enabled: computed(() => props.visible),
  getNextCursor: (page) => page.nextBeforeId,
})
const redemptions = computed(() => redemptionPage.value?.redemptions ?? [])

// 列表初载失败只走 toast（空态表格 + 工具栏刷新即重试入口）。
useFailureToast(initialError, feedback.redemptionsLoadFailed, {
  shouldFireOnMount: false,
  resolveError: () => redemptionListQuery.error.value,
})

watch([() => props.visible, () => props.rewardCode.id], ([visible]) => {
  pagination.reset()
  clearExpansion()
  if (!visible) {
    void queryClient.removeQueries({ queryKey: [...REDEMPTION_LIST_QUERY_ROOT] })
  }
})

onBeforeUnmount(() => {
  activeDetailRequest?.abort()
})

function refreshRedemptions() {
  // Same-key refetch: stale rows stay mounted with the table loading state.
  void refreshRedemptionList()
}

function previousPage() {
  pagination.retreat()
  clearExpansion()
}

function nextPage() {
  pagination.advance()
  clearExpansion()
}

function expandRedemption(event: AdminTableRowExpandEvent<RewardCodeRedemption>) {
  selectedRedemptionId.value = event.data.id
  expandedRows.value = { [String(event.data.id)]: true }
  void loadRedemptionDetail(event.data.id)
}

function collapseRedemption(event: AdminTableRowCollapseEvent) {
  if ((event.data as RewardCodeRedemption).id === selectedRedemptionId.value) clearExpansion()
}

async function loadRedemptionDetail(redemptionId: number) {
  const epoch = ++detailRequestEpoch
  activeDetailRequest?.abort()
  const controller = new AbortController()
  activeDetailRequest = controller
  detailLoading.value = true
  selectedDetail.value = null
  try {
    const detail = await resource.get(props.rewardCode.id, redemptionId, {
      signal: controller.signal,
    })
    if (
      controller.signal.aborted ||
      epoch !== detailRequestEpoch ||
      selectedRedemptionId.value !== redemptionId
    ) {
      return
    }
    selectedDetail.value = detail
  } catch (error) {
    // 行内展开锚点已删除：失败只走 toast。
    if (
      !controller.signal.aborted &&
      epoch === detailRequestEpoch &&
      selectedRedemptionId.value === redemptionId
    ) {
      feedback.redemptionDetailFailed(error)
    }
  } finally {
    if (
      !controller.signal.aborted &&
      epoch === detailRequestEpoch &&
      selectedRedemptionId.value === redemptionId
    ) {
      detailLoading.value = false
    }
  }
}

function clearExpansion() {
  detailRequestEpoch += 1
  activeDetailRequest?.abort()
  selectedRedemptionId.value = null
  selectedDetail.value = null
  detailLoading.value = false
  expandedRows.value = {}
}
</script>
