import { computed, ref, watch } from 'vue'
import { useQueryClient } from '@tanstack/vue-query'
import { useFailureToast } from '@/core/composables/useFailureToast'
import { useAdminCursorCollection } from '../../../shared'
import { useRewardCodesFeedback } from './useRewardCodesFeedback'
import {
  createAdminRewardCodeResource,
  REWARD_CODE_STATUS_FILTER_ALL,
  type AdminRewardCodeResource,
  type RewardCodePage,
  type RewardCodeStatusFilter,
  type SearchRewardCodesInput,
} from './resource'

const REWARD_CODE_LIST_QUERY_ROOT = ['admin', 'credits', 'reward-codes'] as const
const REWARD_CODE_LIST_CRITERIA_INDEX = REWARD_CODE_LIST_QUERY_ROOT.length

const REWARD_CODE_STATUS_OPTIONS: ReadonlyArray<{
  label: string
  value: RewardCodeStatusFilter
}> = [
  { label: '全部状态', value: REWARD_CODE_STATUS_FILTER_ALL },
  { label: '可兑换', value: 'enabled' },
  { label: '已停用', value: 'disabled' },
  { label: '已过期', value: 'expired' },
  { label: '已耗尽', value: 'exhausted' },
]

export function useRewardCodeList(options: { resource?: AdminRewardCodeResource } = {}) {
  const resource = options.resource ?? createAdminRewardCodeResource()
  const feedback = useRewardCodesFeedback()
  const searchKeyword = ref('')
  const appliedSearchKeyword = ref('')
  const statusFilter = ref<RewardCodeStatusFilter>(REWARD_CODE_STATUS_FILTER_ALL)
  const queryClient = useQueryClient()

  // 游标集合生命周期收敛于 useAdminCursorCollection：筛选/分页状态喂给 query key，
  // 翻页与刷新只改状态。
  const {
    data: rewardCodePage,
    loading,
    initialError,
    backgroundError,
    pagination,
    previousPage,
    nextPage,
    query: rewardCodeListQuery,
    queryKey: rewardCodeListKey,
  } = useAdminCursorCollection({
    buildKey: (beforeId) =>
      [
        ...REWARD_CODE_LIST_QUERY_ROOT,
        {
          query: appliedSearchKeyword.value,
          status:
            statusFilter.value === REWARD_CODE_STATUS_FILTER_ALL ? undefined : statusFilter.value,
          beforeId,
        },
      ] as const,
    queryFn: ({ queryKey, signal }) =>
      resource.search({
        ...(queryKey[REWARD_CODE_LIST_CRITERIA_INDEX] as Omit<
          SearchRewardCodesInput,
          'signal' | 'limit'
        >),
        signal,
      }),
    getNextCursor: (page) => page.nextBeforeId,
  })

  useFailureToast(initialError, feedback.loadFailed, {
    shouldFireOnMount: false,
    resolveError: () => rewardCodeListQuery.error.value,
  })

  const rewardCodes = computed(() => rewardCodePage.value?.rewardCodes ?? [])
  const totalRewardCodes = computed(() => rewardCodePage.value?.total ?? 0)
  const totalRewardCodesLabel = computed(() => '共 ' + totalRewardCodes.value + ' 个兑换码')
  const emptyTitle = computed(() =>
    appliedSearchKeyword.value.trim() || statusFilter.value !== REWARD_CODE_STATUS_FILTER_ALL
      ? '未找到匹配的兑换码'
      : '暂无兑换码',
  )

  function updateRewardCodes(update: (page: RewardCodePage) => RewardCodePage) {
    queryClient.setQueryData(rewardCodeListKey.value, (current: RewardCodePage | undefined) =>
      current ? update(current) : current,
    )
  }

  function refreshRewardCodes() {
    // Same-key refetch: stale rows stay mounted with the table loading state,
    // matching the previous background-refresh behavior.
    void rewardCodeListQuery.refetch()
  }

  function resetCriteriaAndReload() {
    // Cursor/filter changes feed the query key and refetch by themselves.
    pagination.reset()
  }

  function searchRewardCodes() {
    appliedSearchKeyword.value = searchKeyword.value
    resetCriteriaAndReload()
  }

  function clearSearch() {
    appliedSearchKeyword.value = ''
    resetCriteriaAndReload()
  }

  function resetStatusFilter() {
    statusFilter.value = REWARD_CODE_STATUS_FILTER_ALL
  }

  watch(statusFilter, () => {
    resetCriteriaAndReload()
  })

  return {
    searchKeyword,
    statusFilter,
    statusOptions: REWARD_CODE_STATUS_OPTIONS,
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
  }
}
