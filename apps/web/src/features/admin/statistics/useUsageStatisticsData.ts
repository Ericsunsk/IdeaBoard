import { computed, ref } from 'vue'
import { useMutation, useQuery } from '@tanstack/vue-query'
import type { AdminStatisticsBackfillState, AdminStatisticsResponse } from '../../../shared/api/adminStatistics'
import type { Channel } from '../../../shared/api/channel'
import { formatDisplayErrorMessage } from '../../../shared/utils/errorMessages'
import { queryKeys } from '../../../shared/query/keys'
import {
  fetchAdminStatistics,
  fetchAllAccounts,
  fetchChannels,
  invalidateAdminStatistics,
  triggerAdminStatisticsBackfill,
  type AdminStatisticsRange,
} from '../../../shared/query/adminQueries'

export type ConfiguredAccountLite = { model_set: string; platform: string; channel_id: number }
type BackfillTriggerResult = { ok: boolean; message?: string }

type RangeOption = { label: string; key: AdminStatisticsRange }

export function useUsageStatisticsData() {
  const timeRange = ref<AdminStatisticsRange>('7d')
  const rangeOptions: RangeOption[] = [
    { label: '周', key: '7d' },
    { label: '月', key: '30d' },
    { label: '年', key: '1y' },
  ]

  const statisticsQuery = useQuery({
    queryKey: computed(() => queryKeys.admin.statistics(timeRange.value)),
    queryFn: () => fetchAdminStatistics(timeRange.value),
    placeholderData: (previous) => previous,
  })

  const channelsQuery = useQuery({
    queryKey: queryKeys.admin.channels(),
    queryFn: fetchChannels,
  })

  const accountsQuery = useQuery({
    queryKey: queryKeys.admin.accounts({ page: 1, page_size: 100 }),
    queryFn: () => fetchAllAccounts({ page: 1, page_size: 100 }),
  })

  const backfillMutation = useMutation({
    mutationFn: () => triggerAdminStatisticsBackfill(timeRange.value),
    onSuccess: async () => {
      await invalidateAdminStatistics(timeRange.value)
    },
  })

  const adminStatistics = computed<AdminStatisticsResponse | null>(() => statisticsQuery.data.value ?? null)
  const configuredAccounts = computed<ConfiguredAccountLite[]>(() => accountsQuery.data.value?.accounts ?? [])
  const configuredChannels = computed<Channel[]>(() => channelsQuery.data.value ?? [])
  const backfillLoading = computed(() => backfillMutation.isPending.value)

  const backfillStatus = computed<AdminStatisticsBackfillState | null>(() => {
    return adminStatistics.value?.backfill_status ?? null
  })

  const needsBackfill = computed(() => {
    const s = backfillStatus.value
    return !!s && s.missing_days > 0
  })

  async function triggerBackfill(): Promise<BackfillTriggerResult> {
    try {
      await backfillMutation.mutateAsync()
      await statisticsQuery.refetch()
      return { ok: true as const }
    } catch (error: unknown) {
      return { ok: false as const, message: formatDisplayErrorMessage(error, '补录失败') }
    }
  }

  return {
    adminStatistics,
    configuredAccounts,
    configuredChannels,
    timeRange,
    rangeOptions,
    backfillStatus,
    needsBackfill,
    backfillLoading,
    triggerBackfill,
  }
}
