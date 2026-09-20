<template>
  <div class="flex w-full flex-col gap-3">
    <!-- 顶栏：左侧标题与状态切换，右侧刷新（与用户管理页/订单页同源） -->
    <div class="flex w-full flex-wrap items-center justify-between gap-2.5">
      <div class="flex min-w-0 flex-1 flex-wrap items-center gap-3">
        <div class="text-base font-bold text-admin-title-color">异常队列</div>
        <div
          class="inline-flex items-center rounded-lg border border-admin-border-soft bg-admin-surface-muted p-0.5"
          role="group"
          aria-label="异常队列视图"
        >
          <button
            v-for="opt in statusOptions"
            :key="opt.value"
            type="button"
            class="rounded-md px-3 py-1 text-xs font-semibold transition-colors cursor-pointer"
            :class="
              status === opt.value
                ? 'bg-white text-admin-text-1 shadow-xs'
                : 'text-admin-text-2 hover:text-admin-text-1'
            "
            @click="status = opt.value"
          >
            {{ opt.label }}
          </button>
        </div>
      </div>

      <div class="flex shrink-0 items-center gap-2">
        <AdminButton
          variant="ghost"
          :class="adminFramedIconButtonClass"
          aria-label="刷新异常队列"
          :loading="loading"
          :disabled="loading"
          @click="refreshFindings"
        >
          <Refresh class="size-4 text-admin-text-2" />
        </AdminButton>
      </div>
    </div>

    <!-- 表格卡片：统一容器 + 告警 + AdminTable + 统一底栏 -->
    <div :class="adminTableCardClass">
      <AdminAlert v-if="backgroundError" class="mx-4 mt-4" severity="secondary">
        <div class="flex flex-wrap items-center justify-between gap-3">
          <span>异常队列刷新失败，当前显示上次加载的数据。</span>
          <AdminButton
            variant="ghost"
            size="small"
            :class="adminTextButtonClass"
            :loading="loading"
            @click="refreshFindings"
          >
            重试刷新
          </AdminButton>
        </div>
      </AdminAlert>

      <AdminAlert v-if="initialError" class="m-4" severity="error">
        <div class="flex flex-wrap items-center justify-between gap-3">
          <span>异常队列加载失败。</span>
          <AdminButton
            variant="outline"
            size="small"
            :class="adminActionButtonClass"
            :loading="loading"
            @click="refreshFindings"
          >
            重新加载异常队列
          </AdminButton>
        </div>
      </AdminAlert>

      <AdminTable
        v-else
        :value="findings"
        :loading="loading"
        data-key="id"
        scrollable
        :frame-class="adminTableFrameClass"
        table-aria-label="账本异常队列"
        scroll-aria-label="账本异常队列横向滚动区域"
        :table-style="adminTableStyle"
        :class="adminTableClass"
      >
        <template #empty>
          <AdminEmptyState :title="emptyTitle" />
        </template>

        <AdminColumn header="异常摘要" :style="{ minWidth: '260px' }">
          <template #body="{ data }">
            <button
              type="button"
              class="group block max-w-[34rem] rounded text-left focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-admin-accent cursor-pointer"
              :aria-label="`查看${getFindingTypePresentation(data.finding_type).title}诊断`"
              @click="openFinding(data)"
            >
              <span class="font-semibold text-admin-text-1 group-hover:text-admin-accent transition-colors">
                {{ getFindingTypePresentation(data.finding_type).title }}
              </span>
              <span class="mt-1 block line-clamp-2 text-xs leading-5 text-admin-text-3">
                {{ getFindingTypePresentation(data.finding_type).summary }}
              </span>
            </button>
          </template>
        </AdminColumn>

        <AdminColumn header="影响对象" :style="{ width: '28%', minWidth: '190px' }">
          <template #body="{ data }">
            <div class="flex min-w-0 flex-col gap-1">
              <span v-if="data.wallet_public_id" class="text-xs font-semibold text-admin-text-1">
                Wallet · {{ data.wallet_display_name || data.wallet_public_id }}
              </span>
              <span
                class="max-w-72 truncate font-mono text-xs text-admin-text-3"
                :title="`${data.subject_type} · ${data.subject_id}`"
              >
                {{ data.subject_type }} · {{ data.subject_id }}
              </span>
            </div>
          </template>
        </AdminColumn>

        <AdminColumn header="风险" :style="{ width: '18%', minWidth: '140px' }">
          <template #body="{ data }">
            <AdminBadge
              :value="getFindingRiskPresentation(data).label"
              :severity="getFindingRiskPresentation(data).severity"
              :class="adminCompactTagClass"
              rounded
            />
          </template>
        </AdminColumn>

        <AdminColumn header="最近发现" :style="{ width: '18%', minWidth: '170px' }">
          <template #body="{ data }">
            <span class="whitespace-nowrap font-mono text-xs text-admin-text-3">
              {{ formatAdminDateTimeWithSeconds(data.last_seen_at) }}
            </span>
          </template>
        </AdminColumn>

        <AdminColumn header="详情" :style="{ width: '64px' }">
          <template #body="{ data }">
            <div class="flex items-center justify-start">
              <button
                type="button"
                :class="adminTableRowActionButtonClass"
                :aria-label="`查看${getFindingTypePresentation(data.finding_type).title}诊断详情`"
                @click="openFinding(data)"
              >
                <Eye class="size-4" />
              </button>
            </div>
          </template>
        </AdminColumn>
      </AdminTable>

      <div :class="adminTableFooterClass">
        <span class="text-xs font-normal text-admin-text-3">
          {{ status === 'open' ? '待处理异常' : '已解决异常' }} ({{ findings.length }})
        </span>
        <div class="flex items-center gap-2">
          <AdminPagerControls
            :current-page="currentPage"
            :total-pages="knownPageCount"
            :prev-disabled="!canRetreat"
            :next-disabled="!canAdvance"
            @prev="previousPage"
            @next="nextPage"
          />
        </div>
      </div>
    </div>
  </div>

  <DialogRoot v-model:open="drawerVisible">
    <DialogPortal>
      <DialogOverlay
        class="fixed inset-0 z-50 bg-black/40 backdrop-blur-xs data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0"
      />
      <DialogContent
        :show-close="false"
        class="fixed inset-y-0 right-0 z-50 flex h-full w-[min(38rem,calc(100vw-1rem))] flex-col border-l border-admin-border-soft bg-white shadow-2xl transition-transform data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:slide-out-to-right data-[state=open]:slide-in-from-right"
      >
        <div class="flex items-center justify-between border-b border-admin-border-soft px-6 py-4">
          <DialogTitle class="text-admin-title-sm font-bold text-admin-title-color">
            异常诊断
          </DialogTitle>
          <DialogDescription class="sr-only">
            查看选中异常的类型说明、影响对象与处置建议。
          </DialogDescription>
          <DialogClose
            class="rounded-lg p-1.5 text-admin-text-subtle hover:bg-admin-surface-hover hover:text-admin-text transition-colors cursor-pointer"
            aria-label="关闭"
          >
            <Xmark class="size-5" />
          </DialogClose>
        </div>
        <div class="flex-1 overflow-y-auto">
          <div v-if="selectedFinding" class="flex min-w-0 flex-col gap-6 px-6 py-5">
            <section class="min-w-0">
              <div class="text-admin-title font-[760] text-admin-title-color">
                {{ getFindingTypePresentation(selectedFinding.finding_type).title }}
              </div>
              <p class="mt-2 mb-0 text-admin-body leading-6 text-admin-text-2">
                {{ getFindingTypePresentation(selectedFinding.finding_type).summary }}
              </p>
              <AdminBadge
                class="mt-3"
                :value="getFindingRiskPresentation(selectedFinding).label"
                :severity="getFindingRiskPresentation(selectedFinding).severity"
                rounded
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
                <div class="mt-1 font-mono text-admin-body font-[680] text-admin-text-1">
                  {{ selectedFinding.occurrence_count }}
                </div>
              </div>
              <div>
                <div class="text-admin-caption font-[700] text-admin-text-3">首次发现</div>
                <div class="mt-1 font-mono text-admin-body text-admin-text-1">
                  {{ formatAdminDateTimeWithSeconds(selectedFinding.first_seen_at) }}
                </div>
              </div>
              <div>
                <div class="text-admin-caption font-[700] text-admin-text-3">最近发现</div>
                <div class="mt-1 font-mono text-admin-body text-admin-text-1">
                  {{ formatAdminDateTimeWithSeconds(selectedFinding.last_seen_at) }}
                </div>
              </div>
              <div v-if="selectedFinding.resolved_at">
                <div class="text-admin-caption font-[700] text-admin-text-3">解决时间</div>
                <div class="mt-1 font-mono text-admin-body text-admin-text-1">
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
                  class="min-w-0 rounded-lg border border-admin-border-soft bg-admin-bg/50 px-3 py-2.5"
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
                class="mt-2 break-all rounded-lg border border-admin-border-soft bg-admin-bg/50 px-3 py-2.5 font-mono text-admin-caption leading-5 text-admin-text-2"
              >
                {{ selectedFinding.fingerprint }}
              </div>
            </section>
          </div>
        </div>
      </DialogContent>
    </DialogPortal>
  </DialogRoot>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { Eye, Refresh, Xmark } from '@iconoir/vue'
import {
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogOverlay,
  DialogPortal,
  DialogRoot,
  DialogTitle,
} from 'reka-ui'
import AdminAlert from '../../shared/components/AdminAlert.vue'
import AdminBadge from '../../shared/components/AdminBadge.vue'
import AdminButton from '../../shared/components/AdminButton.vue'
import AdminColumn from '../../shared/components/AdminColumn.vue'
import AdminEmptyState from '../../shared/components/AdminEmptyState.vue'
import AdminPagerControls from '../../shared/components/AdminPagerControls.vue'
import AdminTable from '../../shared/components/AdminTable.vue'
import { useAdminCollectionQuery } from '../../shared/composables/useAdminCollectionQuery'
import { useCursorPagination } from '../../shared/composables/useCursorPagination'
import {
  adminActionButtonClass,
  adminCompactTagClass,
  adminFramedIconButtonClass,
  adminTableCardClass,
  adminTableClass,
  adminTableFooterClass,
  adminTableFrameClass,
  adminTableRowActionButtonClass,
  adminTableStyle,
  adminTextButtonClass,
} from '../../shared/lib/uiClasses'
import { formatAdminDateTimeWithSeconds } from '../shared/format'
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
  getNextCursor: (page) => page.next_before_id,
  pagination,
  refetchInterval: computed(() => (status.value === 'open' ? 5_000 : 30_000)),
})
const findings = computed(() => findingPage.value?.findings ?? [])
const currentPage = pagination.currentPage
const knownPageCount = pagination.knownPageCount
const canAdvance = pagination.canAdvance
const canRetreat = pagination.canRetreat
const emptyTitle = computed(() => (status.value === 'open' ? '暂无待处理异常' : '暂无已解决异常'))
const detailEntries = computed(() =>
  selectedFinding.value ? formatFindingDetailsEntries(selectedFinding.value.details) : [],
)

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
