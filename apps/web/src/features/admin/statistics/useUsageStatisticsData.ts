import { ref, computed, type Ref } from 'vue'
import accountApi from '../../../shared/api/account'
import { adminStatisticsApi, type AdminStatisticsBackfillState } from '../../../shared/api/adminStatistics'
import type { AdminStatisticsResponse } from '../../../shared/api/adminStatistics'
import channelApi, { type Channel } from '../../../shared/api/channel'
import { extractSafeApiErrorMessage } from '../../../shared/utils/errorMessages'

export type ConfiguredAccountLite = { model_set: string; platform: string; channel_id: number }
type BackfillTriggerResult = { ok: boolean; message?: string }

export function useUsageStatisticsData() {
  const adminStatistics = ref<AdminStatisticsResponse | null>(null)
  const configuredAccounts = ref<ConfiguredAccountLite[]>([])
  const configuredChannels = ref<Channel[]>([])
  const timeRange = ref('7d')
  const backfillLoading = ref(false)
  const rangeOptions = [
    { label: '周', key: '7d' },
    { label: '月', key: '30d' },
    { label: '年', key: '1y' },
  ]
  const ACCOUNT_PAGE_SIZE = 100

  const backfillStatus = computed<AdminStatisticsBackfillState | null>(() => {
    return adminStatistics.value?.backfill_status ?? null
  })

  const needsBackfill = computed(() => {
    const s = backfillStatus.value
    return !!s && s.missing_days > 0
  })

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
        adminStatisticsApi.get(activeRange as '7d' | '30d' | '1y'),
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

  async function triggerBackfill(): Promise<BackfillTriggerResult> {
    backfillLoading.value = true
    try {
      const res = await adminStatisticsApi.backfill(timeRange.value as '7d' | '30d' | '1y')
      if (res.code === 0) {
        await loadStatistics()
        return { ok: true as const }
      }
      return { ok: false as const, message: res.msg || '补录失败' }
    } catch (error: unknown) {
      return { ok: false as const, message: extractSafeApiErrorMessage(error, '补录失败') }
    } finally {
      backfillLoading.value = false
    }
  }

  return {
    adminStatistics,
    configuredAccounts,
    configuredChannels,
    timeRange,
    rangeOptions,
    loadStatistics,
    backfillStatus,
    needsBackfill,
    backfillLoading,
    triggerBackfill,
  }
}
