<template>
  <AdminDialogShell
    :visible="visible"
    :header="`${rewardCode.displayName} · 兑换记录`"
    size="wide"
    content-layout="scrollable"
    @update:visible="emit('update:visible', $event)"
  >
    <AdminTablePanel>
      <template #toolbar>
        <AdminResourceToolbar>
          <template #start>
            <div class="min-w-0">
              <div class="text-admin-title-sm font-[740] text-admin-title-color">兑换历史</div>
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
            <Button
              text
              :class="iconButtonClass"
              :loading="loading"
              aria-label="刷新兑换记录"
              @click="refreshRedemptions"
            >
              <template #icon><Refresh class="size-4 text-admin-text-2" /></template>
            </Button>
          </template>
        </AdminResourceToolbar>
      </template>

      <Message v-if="initialError" severity="error" :closable="false">
        <div class="flex flex-wrap items-center justify-between gap-3">
          <span>兑换记录加载失败，请重试。</span>
          <Button
            label="重试加载兑换记录"
            text
            :class="adminTextButtonClass"
            :loading="loading"
            @click="loadRedemptions()"
          />
        </div>
      </Message>

      <Message v-if="backgroundError" severity="secondary" :closable="false">
        <div class="flex flex-wrap items-center justify-between gap-3">
          <span>刷新兑换记录失败，正在显示上次成功的数据。</span>
          <Button
            label="重试刷新兑换记录"
            text
            :class="adminTextButtonClass"
            :loading="loading"
            @click="refreshRedemptions"
          />
        </div>
      </Message>

      <DataTableFrame
        v-if="!initialError"
        v-model:expanded-rows="expandedRows"
        :value="redemptions"
        :loading="loading"
        data-key="id"
        scrollable
        table-aria-label="兑换记录列表"
        scroll-aria-label="兑换记录列表横向滚动区域"
        empty-title="暂无兑换记录"
        :table-style="{ minWidth: '620px' }"
        @row-expand="expandRedemption"
        @row-collapse="collapseRedemption"
      >
        <Column expander :style="{ width: '52px' }" />
        <Column field="userPublicId" header="用户" :style="{ minWidth: '240px' }">
          <template #body="{ data }">
            <span class="font-semibold text-admin-text-1">{{ data.displayName }}</span>
            <span class="block text-admin-caption text-admin-text-3">{{ data.userPublicId }}</span>
          </template>
        </Column>
        <Column field="creditedAmount" header="获得 Idea" :style="{ width: '180px' }">
          <template #body="{ data }">{{ formatIdeaAmount(data.creditedAmount) }}</template>
        </Column>
        <Column field="redeemedAt" header="兑换时间" :style="{ width: '220px' }">
          <template #body="{ data }">{{ formatAdminDateTime(data.redeemedAt) }}</template>
        </Column>

        <template #expansion="{ data }">
          <div class="border-y border-admin-border-soft bg-ib-surface-muted px-5 py-4">
            <div
              v-if="detailLoading && selectedRedemptionId === data.id"
              class="flex min-h-24 items-center justify-center"
              aria-label="加载兑换详情"
            >
              <ProgressSpinner class="h-7 w-7 text-admin-accent" stroke-width="4" />
            </div>
            <Message
              v-else-if="detailError && selectedRedemptionId === data.id"
              severity="error"
              :closable="false"
            >
              <div class="flex flex-wrap items-center justify-between gap-3">
                <span>兑换详情加载失败。</span>
                <Button
                  label="重试兑换详情"
                  text
                  :class="adminTextButtonClass"
                  :loading="detailLoading"
                  @click="loadRedemptionDetail(data.id)"
                />
              </div>
            </Message>
            <dl
              v-else-if="selectedDetail?.id === data.id"
              class="grid gap-x-6 gap-y-4 sm:grid-cols-3"
            >
              <div>
                <dt class="text-admin-caption font-[700] text-admin-text-3">Redemption ID</dt>
                <dd
                  class="mt-1 mb-0 break-all font-mono text-admin-body text-admin-text-1 select-all"
                >
                  {{ selectedDetail?.id }}
                </dd>
              </div>
              <div>
                <dt class="text-admin-caption font-[700] text-admin-text-3">
                  Credits Transaction ID
                </dt>
                <dd
                  class="mt-1 mb-0 break-all font-mono text-admin-body text-admin-text-1 select-all"
                >
                  {{ selectedDetail?.creditTransactionId }}
                </dd>
              </div>
              <div>
                <dt class="text-admin-caption font-[700] text-admin-text-3">masked Code hint</dt>
                <dd
                  class="mt-1 mb-0 break-all font-mono text-admin-body text-admin-text-1 select-all"
                >
                  {{ selectedDetail?.maskedCodeHint }}
                </dd>
              </div>
            </dl>
          </div>
        </template>
      </DataTableFrame>
    </AdminTablePanel>
  </AdminDialogShell>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { Refresh } from '@iconoir/vue'
import Button from 'primevue/button'
import Column from 'primevue/column'
import type { DataTableRowCollapseEvent, DataTableRowExpandEvent } from 'primevue/datatable'
import Message from 'primevue/message'
import ProgressSpinner from 'primevue/progressspinner'
import DataTableFrame from '../../../../shared/components/DataTableFrame.vue'
import { formatIdeaAmount } from '../../shared/lib/ideaCurrency'
import AdminDialogShell from '../../shared/components/AdminDialogShell.vue'
import AdminPagerControls from '../../shared/components/AdminPagerControls.vue'
import AdminResourceToolbar from '../../shared/components/AdminResourceToolbar.vue'
import AdminTablePanel from '../../shared/components/AdminTablePanel.vue'
import { useCollectionState } from '../../shared/composables/useCollectionState'
import { useCursorPagination } from '../../shared/composables/useCursorPagination'
import { formatAdminDateTime } from '../format'
import { adminTextButtonClass } from '../../shared/lib/uiClasses'
import type { RewardCode } from './resource'
import {
  createRewardCodeRedemptionResource,
  type RewardCodeRedemption,
  type RewardCodeRedemptionDetail,
} from './rewardCodeRedemptionResource'

const props = defineProps<{
  visible: boolean
  rewardCode: RewardCode
}>()

const emit = defineEmits<{
  'update:visible': [visible: boolean]
}>()

const resource = createRewardCodeRedemptionResource()
const expandedRows = ref<Record<string, boolean>>({})
const selectedRedemptionId = ref<number | null>(null)
const selectedDetail = ref<RewardCodeRedemptionDetail | null>(null)
const detailLoading = ref(false)
const detailError = ref(false)
let activeDetailRequest: AbortController | undefined
let detailRequestEpoch = 0
let pagination: ReturnType<typeof useCursorPagination<number>>
const collection = useCollectionState({
  load: (signal) =>
    resource.list(props.rewardCode.id, {
      beforeId: pagination.cursor.value,
      signal,
    }),
  isEmpty: (page) => page.redemptions.length === 0,
})
const { data: redemptionPage, loading, initialError, backgroundError } = collection
pagination = useCursorPagination<number>({ pending: loading })
const redemptions = computed(() => redemptionPage.value?.redemptions ?? [])

watch(
  [() => props.visible, () => props.rewardCode.id],
  ([visible]) => {
    resetDialogState()
    if (!visible) return
    void loadRedemptions()
  },
  { immediate: true },
)

onBeforeUnmount(() => {
  collection.cancel()
  activeDetailRequest?.abort()
})

async function loadRedemptions({ preserveData = false }: { preserveData?: boolean } = {}) {
  const page = await collection.reload({ preserveData })
  if (page) pagination.setNextCursor(page.nextBeforeId)
}

function previousPage() {
  if (!pagination.retreat()) return
  clearExpansion()
  void loadRedemptions()
}

function nextPage() {
  if (!pagination.advance()) return
  clearExpansion()
  void loadRedemptions()
}

function refreshRedemptions() {
  void loadRedemptions({ preserveData: true })
}

function expandRedemption(event: DataTableRowExpandEvent<RewardCodeRedemption>) {
  selectedRedemptionId.value = event.data.id
  expandedRows.value = { [String(event.data.id)]: true }
  void loadRedemptionDetail(event.data.id)
}

function collapseRedemption(event: DataTableRowCollapseEvent) {
  if ((event.data as RewardCodeRedemption).id === selectedRedemptionId.value) clearExpansion()
}

async function loadRedemptionDetail(redemptionId: number) {
  const epoch = ++detailRequestEpoch
  activeDetailRequest?.abort()
  const controller = new AbortController()
  activeDetailRequest = controller
  detailLoading.value = true
  detailError.value = false
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
  } catch {
    if (
      !controller.signal.aborted &&
      epoch === detailRequestEpoch &&
      selectedRedemptionId.value === redemptionId
    ) {
      detailError.value = true
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
  detailError.value = false
  expandedRows.value = {}
}

function resetDialogState() {
  collection.reset()
  pagination.reset()
  clearExpansion()
}
</script>
