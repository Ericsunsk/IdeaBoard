<template>
  <AdminTablePanel>
    <template #toolbar>
      <AdminResourceToolbar>
        <template #start>
          <div class="min-w-0">
            <div class="text-admin-title-sm font-[740] text-admin-title-color">异常队列</div>
          </div>
        </template>
        <template #filters>
          <SelectButton
            v-model="status"
            :options="statusOptions"
            option-label="label"
            option-value="value"
            :allow-empty="false"
            size="small"
            aria-label="异常队列视图"
          />
        </template>
        <template #actions="{ iconButtonClass }">
          <Button
            text
            :class="iconButtonClass"
            aria-label="刷新异常队列"
            :loading="loading"
            @click="refreshFindings"
          >
            <template #icon><Refresh class="size-4" /></template>
          </Button>
        </template>
        <template #pager>
          <AdminPagerControls
            :current-page="currentPage"
            :total-pages="knownPageCount"
            :prev-disabled="!canRetreat"
            :next-disabled="!canAdvance"
            @prev="previousPage"
            @next="nextPage"
          />
        </template>
      </AdminResourceToolbar>
    </template>

    <Message v-if="backgroundError" class="m-4" severity="secondary" :closable="false">
      <div class="flex flex-wrap items-center justify-between gap-3">
        <span>异常队列刷新失败，当前显示上次加载的数据。</span>
        <Button
          label="重试刷新"
          severity="secondary"
          size="small"
          text
          :class="adminTextButtonClass"
          :loading="loading"
          @click="refreshFindings"
        />
      </div>
    </Message>

    <Message v-if="initialError" class="m-5" severity="error" :closable="false">
      <div class="flex flex-wrap items-center justify-between gap-3">
        <span>异常队列加载失败。</span>
        <Button
          label="重新加载异常队列"
          :class="adminActionButtonClass"
          severity="secondary"
          outlined
          :loading="loading"
          @click="refreshFindings"
        />
      </div>
    </Message>

    <DataTableFrame
      v-else
      :value="findings"
      data-key="id"
      :loading="loading"
      scrollable
      table-style="min-width: 760px; width: 100%"
      table-aria-label="账本异常队列"
    >
      <template #empty>
        <AdminEmptyState :title="status === 'open' ? '暂无待处理异常' : '暂无已解决异常'" />
      </template>

      <Column header="异常摘要" style="width: 38%; min-width: 260px">
        <template #body="{ data }">
          <button
            type="button"
            class="group block max-w-[34rem] rounded-[var(--radius-ib-control)] text-left focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-admin-accent"
            :aria-label="`查看${getFindingTypePresentation(data.finding_type).title}诊断`"
            @click="openFinding(data)"
          >
            <span class="font-[720] text-admin-text-1 group-hover:text-admin-accent">
              {{ getFindingTypePresentation(data.finding_type).title }}
            </span>
            <span class="mt-1 block line-clamp-2 text-admin-caption leading-5 text-admin-text-3">
              {{ getFindingTypePresentation(data.finding_type).summary }}
            </span>
          </button>
        </template>
      </Column>
      <Column header="影响对象" style="width: 28%; min-width: 190px">
        <template #body="{ data }">
          <div class="flex min-w-0 flex-col gap-1">
            <span v-if="data.wallet_public_id" class="text-admin-body font-[700] text-admin-text-1">
              Wallet · {{ data.wallet_display_name || data.wallet_public_id }}
            </span>
            <span
              class="max-w-72 truncate text-admin-caption text-admin-text-2"
              :title="`${data.subject_type} · ${data.subject_id}`"
            >
              {{ data.subject_type }} · {{ data.subject_id }}
            </span>
          </div>
        </template>
      </Column>
      <Column header="风险" style="width: 18%; min-width: 140px">
        <template #body="{ data }">
          <Tag
            :value="getFindingRiskPresentation(data).label"
            :severity="getFindingRiskPresentation(data).severity"
            :class="adminCompactTagClass"
          />
        </template>
      </Column>
      <Column header="最近发现" style="width: 16%; min-width: 170px">
        <template #body="{ data }">
          <span class="whitespace-nowrap">{{
            formatAdminDateTimeWithSeconds(data.last_seen_at)
          }}</span>
        </template>
      </Column>
    </DataTableFrame>
  </AdminTablePanel>

  <Drawer v-model:visible="drawerVisible" position="right" header="异常诊断" :pt="drawerPt">
    <div v-if="selectedFinding" class="flex min-w-0 flex-col gap-6 px-5 py-5">
      <section class="min-w-0">
        <div class="text-admin-title font-[760] text-admin-title-color">
          {{ getFindingTypePresentation(selectedFinding.finding_type).title }}
        </div>
        <p class="mt-2 mb-0 text-admin-body leading-6 text-admin-text-2">
          {{ getFindingTypePresentation(selectedFinding.finding_type).summary }}
        </p>
        <Tag
          class="mt-3"
          :value="getFindingRiskPresentation(selectedFinding).label"
          :severity="getFindingRiskPresentation(selectedFinding).severity"
        />
      </section>

      <section class="grid gap-x-5 gap-y-4 border-y border-admin-border-soft py-5 sm:grid-cols-2">
        <div>
          <div class="text-admin-caption font-[700] text-admin-text-3">影响对象</div>
          <div class="mt-1 break-all text-admin-body font-[680] text-admin-text-1">
            {{ selectedFinding.subject_type }} · {{ selectedFinding.subject_id }}
          </div>
        </div>
        <div>
          <div class="text-admin-caption font-[700] text-admin-text-3">出现次数</div>
          <div class="mt-1 text-admin-body font-[680] text-admin-text-1">
            {{ selectedFinding.occurrence_count }}
          </div>
        </div>
        <div>
          <div class="text-admin-caption font-[700] text-admin-text-3">首次发现</div>
          <div class="mt-1 text-admin-body text-admin-text-1">
            {{ formatAdminDateTimeWithSeconds(selectedFinding.first_seen_at) }}
          </div>
        </div>
        <div>
          <div class="text-admin-caption font-[700] text-admin-text-3">最近发现</div>
          <div class="mt-1 text-admin-body text-admin-text-1">
            {{ formatAdminDateTimeWithSeconds(selectedFinding.last_seen_at) }}
          </div>
        </div>
        <div v-if="selectedFinding.resolved_at">
          <div class="text-admin-caption font-[700] text-admin-text-3">解决时间</div>
          <div class="mt-1 text-admin-body text-admin-text-1">
            {{ formatAdminDateTimeWithSeconds(selectedFinding.resolved_at) }}
          </div>
        </div>
      </section>

      <section v-if="selectedFinding.wallet_public_id">
        <div class="text-admin-caption font-[700] text-admin-text-3">关联 Wallet</div>
        <div class="mt-2 text-admin-body font-[680] text-admin-text-1">
          Wallet · {{ selectedFinding.wallet_display_name || selectedFinding.wallet_public_id }}
        </div>
      </section>

      <section class="min-w-0">
        <div class="text-admin-caption font-[700] text-admin-text-3">完整 Details</div>
        <dl v-if="detailEntries.length > 0" class="mt-2 grid min-w-0 gap-2">
          <div
            v-for="entry in detailEntries"
            :key="entry.key"
            class="min-w-0 rounded-[var(--radius-ib-control)] border border-admin-border-soft bg-ib-surface-muted px-3 py-2.5"
          >
            <dt class="text-admin-caption font-[700] text-admin-text-3">{{ entry.key }}</dt>
            <dd
              class="mt-1 mb-0 whitespace-pre-wrap break-all font-mono text-admin-caption leading-5 text-admin-text-1"
            >
              {{ entry.value }}
            </dd>
          </div>
        </dl>
        <div v-else class="mt-2 text-admin-body text-admin-text-3">无附加 details</div>
      </section>

      <section class="min-w-0">
        <div class="text-admin-caption font-[700] text-admin-text-3">Fingerprint</div>
        <div
          class="mt-2 break-all rounded-[var(--radius-ib-control)] bg-ib-surface-muted px-3 py-2.5 font-mono text-admin-caption leading-5 text-admin-text-2"
        >
          {{ selectedFinding.fingerprint }}
        </div>
      </section>
    </div>
  </Drawer>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useAdminCollectionQuery } from '../../shared/composables/useAdminCollectionQuery'
import { Refresh } from '@iconoir/vue'
import Button from 'primevue/button'
import Column from 'primevue/column'
import Drawer from 'primevue/drawer'
import Message from 'primevue/message'
import SelectButton from 'primevue/selectbutton'
import Tag from 'primevue/tag'
import DataTableFrame from '../../../../shared/components/DataTableFrame.vue'
import AdminEmptyState from '../../shared/components/AdminEmptyState.vue'
import AdminPagerControls from '../../shared/components/AdminPagerControls.vue'
import AdminResourceToolbar from '../../shared/components/AdminResourceToolbar.vue'
import AdminTablePanel from '../../shared/components/AdminTablePanel.vue'
import { useCursorPagination } from '../../shared/composables/useCursorPagination'
import {
  adminActionButtonClass,
  adminCompactTagClass,
  adminTextButtonClass,
} from '../../shared/lib/uiClasses'
import { formatAdminDateTimeWithSeconds } from '../format'
import {
  createAdminCreditFindingQueueResource,
  type AdminCreditFinding,
  type AdminCreditFindingStatus,
} from './findingQueueResource'
import {
  formatFindingDetailsEntries,
  getFindingRiskPresentation,
  getFindingTypePresentation,
} from './reconciliationFindingPresentation'

const statusOptions = [
  { label: '待处理', value: 'open' },
  { label: '已解决', value: 'resolved' },
] satisfies Array<{ label: string; value: AdminCreditFindingStatus }>

const status = ref<AdminCreditFindingStatus>('open')
const selectedFinding = ref<AdminCreditFinding | null>(null)
const drawerVisible = ref(false)
let pagination: ReturnType<typeof useCursorPagination<number>>
const resource = createAdminCreditFindingQueueResource()
// TanStack Query owns the list lifecycle (same pattern as the orders pilot).
// The previous recursive refresh timer folds into `refetchInterval`, which
// follows the status cadence (5s open / 30s resolved) reactively.
pagination = useCursorPagination<number>({ pending: () => loading.value })
const FINDING_LIST_QUERY_ROOT = ['admin', 'credits', 'findings'] as const
const FINDING_LIST_STATUS_INDEX = FINDING_LIST_QUERY_ROOT.length
const FINDING_LIST_CURSOR_INDEX = FINDING_LIST_QUERY_ROOT.length + 1
const findingListKey = computed(
  () => [...FINDING_LIST_QUERY_ROOT, status.value, pagination.cursor.value] as const,
)
const {
  data: findingPage,
  loading,
  initialError,
  backgroundError,
  refresh: refreshFindingList,
} = useAdminCollectionQuery({
  queryKey: findingListKey,
  queryFn: ({ queryKey, signal }) =>
    resource.list({
      status: queryKey[FINDING_LIST_STATUS_INDEX] as AdminCreditFindingStatus,
      beforeId: queryKey[FINDING_LIST_CURSOR_INDEX] as number | undefined,
      signal,
    }),
  refetchInterval: computed(() => (status.value === 'open' ? 5_000 : 30_000)),
  getNextCursor: (page) => page.next_before_id,
  pagination,
})
const findings = computed(() => findingPage.value?.findings ?? [])
const currentPage = pagination.currentPage
const knownPageCount = pagination.knownPageCount
const canAdvance = pagination.canAdvance
const canRetreat = pagination.canRetreat
const detailEntries = computed(() =>
  selectedFinding.value ? formatFindingDetailsEntries(selectedFinding.value.details) : [],
)
const drawerPt = {
  root: {
    class: 'w-[min(38rem,calc(100vw-1rem))] border-l border-admin-border bg-ib-surface',
  },
  header: { class: 'border-b border-admin-border-soft px-5 py-4' },
  content: { class: 'p-0' },
} as const

watch(status, () => {
  pagination.reset()
  drawerVisible.value = false
  selectedFinding.value = null
})

function refreshFindings() {
  // Same-key refetch: stale rows stay mounted with the table loading state.
  void refreshFindingList()
}

function nextPage() {
  if (!pagination.advance()) return
}

function previousPage() {
  if (!pagination.retreat()) return
}

function openFinding(finding: AdminCreditFinding) {
  selectedFinding.value = finding
  drawerVisible.value = true
}
</script>
