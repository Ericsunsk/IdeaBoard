import type {
  AdminStatisticsBackfillState,
  AdminStatisticsResponse,
} from '../../../shared/api/adminStatistics'
import type { Channel } from '../../../shared/api/channel'
import { formatDisplayErrorMessage } from '../../../shared/utils/errorMessages'
import {
  getGetAdminStatisticsQueryKey,
  useGetAdminStatistics,
} from '../../../shared/api/generated-query/admin-statistics/admin-statistics'
import { useListAdminChannels } from '../../../shared/api/generated-query/admin-channels/admin-channels'
import {
  getListAdminAccountsQueryKey,
  listAdminAccounts,
} from '../../../shared/api/generated-query/admin-accounts/admin-accounts'
import type { ListAdminAccountsParams } from '../../../shared/api/generated-query/model'
import {
  triggerAdminStatisticsBackfill,
  type AdminStatisticsRange,
} from '../../../shared/query/adminQueries'

export type ConfiguredAccountLite = { model_set: string; platform: string; channel_id: number }
type ConfiguredAccountsResult = { accounts: ConfiguredAccountLite[]; total: number }
type BackfillTriggerResult = { ok: boolean; message?: string }

type RangeOption = { label: string; key: AdminStatisticsRange }
const accountListParams = { page: 1, page_size: 100 } as const

function requireAccountListData(response: Awaited<ReturnType<typeof listAdminAccounts>>) {
  if (!response.data) throw new Error('获取账号列表失败')
  return response.data
}

async function listAllAdminAccounts(
  params: ListAdminAccountsParams,
): Promise<ConfiguredAccountsResult> {
  const pageSize = params.page_size ?? 100
  const firstPage = requireAccountListData(
    await listAdminAccounts({ ...params, page: 1, page_size: pageSize }),
  )
  const accounts: ConfiguredAccountLite[] = [...firstPage.list]
  const total = firstPage.total
  let page = 2

  while (accounts.length < total) {
    const data = requireAccountListData(
      await listAdminAccounts({ ...params, page, page_size: pageSize }),
    )
    if (data.list.length === 0) break
    accounts.push(...data.list)
    page += 1
  }

  return { accounts, total }
}

export function useUsageStatisticsData() {
  const queryClient = useQueryClient()
  const timeRange = ref<AdminStatisticsRange>('7d')
  const rangeOptions: RangeOption[] = [
    { label: '周', key: '7d' },
    { label: '月', key: '30d' },
    { label: '年', key: '1y' },
  ]

  const statisticsParams = computed(() => ({ range: timeRange.value }))
  const statisticsQuery = useGetAdminStatistics(statisticsParams, {
    query: {
      placeholderData: (previous) => previous,
    },
  })

  const channelsQuery = useListAdminChannels()

  const accountsQuery = useQuery({
    queryKey: [...getListAdminAccountsQueryKey(accountListParams), 'all-pages'] as const,
    queryFn: () => listAllAdminAccounts(accountListParams),
  })

  const backfillMutation = useMutation({
    mutationFn: () => triggerAdminStatisticsBackfill(timeRange.value),
    onSuccess: async () => {
      await queryClient.invalidateQueries({
        queryKey: getGetAdminStatisticsQueryKey(statisticsParams.value),
      })
    },
  })

  const adminStatistics = computed<AdminStatisticsResponse | null>(
    () => statisticsQuery.data.value?.data ?? null,
  )
  const configuredAccounts = computed<ConfiguredAccountLite[]>(
    () => accountsQuery.data.value?.accounts ?? [],
  )
  const configuredChannels = computed<Channel[]>(() => channelsQuery.data.value?.data?.list ?? [])
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
