import { ref, type Ref } from 'vue'
import accountApi from '../../../shared/api/account'
import type { AdminStatisticsResponse } from '../../../shared/api/account'
import channelApi, { type Channel } from '../../../shared/api/channel'

export type ConfiguredAccountLite = { model_set: string; platform: string; channel_id: number }

export function useUsageStatisticsData() {
  const adminStatistics = ref<AdminStatisticsResponse | null>(null)
  const configuredAccounts = ref<ConfiguredAccountLite[]>([])
  const configuredChannels = ref<Channel[]>([])
  const timeRange = ref('7d')
  const rangeOptions = [
    { label: '周', key: '7d' },
    { label: '月', key: '30d' },
    { label: '年', key: '1y' },
  ]
  const ACCOUNT_PAGE_SIZE = 100

  async function loadAllAccounts() {
    const all: ConfiguredAccountLite[] = []
    let page = 1
    let total = Number.POSITIVE_INFINITY

    while (true) {
      const res = await accountApi.listAccounts({ page, page_size: ACCOUNT_PAGE_SIZE })
      if (res.code !== 0) break
      const list = (res.data?.list || []) as ConfiguredAccountLite[]
      total = res.data?.total ?? total
      all.push(...list)
      if (all.length >= total || list.length === 0) break
      page += 1
    }

    return all
  }

  async function loadStatistics(targetRange?: Ref<string> | string) {
    const activeRange = typeof targetRange === 'string' ? targetRange : (targetRange?.value || timeRange.value)
    try {
      const [statsRes, channelsRes, firstAccountsRes] = await Promise.all([
        accountApi.getAdminStatistics(activeRange as '7d' | '30d' | '1y'),
        channelApi.list(),
        accountApi.listAccounts({ page: 1, page_size: ACCOUNT_PAGE_SIZE }),
      ])
      adminStatistics.value = statsRes.code === 0 ? (statsRes.data || null) : null
      configuredChannels.value = channelsRes.code === 0 ? (channelsRes.data?.list || []) : []
      configuredAccounts.value = firstAccountsRes.code === 0 ? ((firstAccountsRes.data?.list || []) as ConfiguredAccountLite[]) : []

      if (firstAccountsRes.code === 0) {
        const firstPage = (firstAccountsRes.data?.list || []) as ConfiguredAccountLite[]
        const total = firstAccountsRes.data?.total || firstPage.length
        if (firstPage.length < total) {
          configuredAccounts.value = await loadAllAccounts()
        }
      }
    } catch {
      adminStatistics.value = null
      configuredAccounts.value = []
      configuredChannels.value = []
    }
  }

  return {
    adminStatistics,
    configuredAccounts,
    configuredChannels,
    timeRange,
    rangeOptions,
    loadStatistics,
  }
}
