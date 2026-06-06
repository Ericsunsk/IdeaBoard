import { computed, ref, watch, type Ref } from 'vue'
import { useGetAdminMonitor } from '../../../../shared/api/generated-query/admin-monitor/admin-monitor'
import { useGetAdminStatistics } from '../../../../shared/api/generated-query/admin-statistics/admin-statistics'
import type { AdminStatistics } from '../../../../shared/api/generated-query/model'
import {
  buildAdminAnomalyItems,
  type AdminAnomalyItem,
  type AdminAnomalySeverity,
} from '../lib/adminAnomalyRules'
import {
  createMockAdminStatistics,
  mockMonitorData,
  mockMonitorStats,
  useAdminMockMode,
} from '../lib/adminMockData'
import { useAdminQueryPolling } from './useAdminQueryPolling'

type UseAdminAnomalyRadarOptions = {
  enabled?: Ref<boolean>
}

type StableAnomalyRecord = {
  item: AdminAnomalyItem
  missingSamples: number
}

const RECOVERY_SAMPLE_COUNT = 3
const severityRank: Record<AdminAnomalySeverity, number> = {
  critical: 0,
  warning: 1,
  info: 2,
}

export function useAdminAnomalyRadar(options: UseAdminAnomalyRadarOptions = {}) {
  const isMockMode = useAdminMockMode()
  const isEnabled = computed(() => options.enabled?.value ?? true)
  const polling = useAdminQueryPolling({
    visibleInterval: 15_000,
    hiddenInterval: 60_000,
    isEnabled: () => isEnabled.value && !isMockMode.value,
  })

  const monitorQuery = useGetAdminMonitor({
    query: {
      enabled: computed(() => isEnabled.value && !isMockMode.value),
      refetchInterval: polling.refetchInterval,
      refetchIntervalInBackground: false,
      refetchOnWindowFocus: false,
    },
  })
  const statisticsQuery = useGetAdminStatistics(
    computed(() => ({ range: '1d' as const })),
    {
      query: {
        enabled: computed(() => isEnabled.value && !isMockMode.value),
        refetchInterval: polling.refetchInterval,
        refetchIntervalInBackground: false,
        refetchOnWindowFocus: false,
        staleTime: 60_000,
      },
    },
  )

  const monitorData = computed(() =>
    isMockMode.value ? mockMonitorData : monitorQuery.data.value?.data?.monitor || null,
  )
  const monitorStats = computed(() =>
    isMockMode.value ? mockMonitorStats : monitorQuery.data.value?.data?.stats || null,
  )
  const statistics = computed<AdminStatistics | null>(() =>
    isMockMode.value ? createMockAdminStatistics('1d') : (statisticsQuery.data.value?.data ?? null),
  )
  const stableRecords = ref(new Map<string, StableAnomalyRecord>())
  const anomalies = ref<AdminAnomalyItem[]>([])

  const rawAnomalies = computed(() => {
    if (!isEnabled.value) return []
    return buildAdminAnomalyItems({
      monitorData: monitorData.value,
      monitorStats: monitorStats.value,
      statistics: statistics.value,
      isMonitorDegraded: false,
      isMonitorUnavailable: !isMockMode.value && monitorQuery.isError.value,
      isStatisticsUnavailable: !isMockMode.value && statisticsQuery.isError.value,
    })
  })

  watch(
    rawAnomalies,
    (nextAnomalies) => {
      stableRecords.value = reconcileStableAnomalies(stableRecords.value, nextAnomalies)
      anomalies.value = sortAnomalies(
        Array.from(stableRecords.value.values(), (record) => record.item),
      )
    },
    { immediate: true },
  )

  const anomalyCount = computed(() => anomalies.value.length)
  const hasAnomalies = computed(() => anomalyCount.value > 0)
  const highestSeverity = computed<AdminAnomalySeverity | null>(
    () => anomalies.value[0]?.severity ?? null,
  )

  return {
    anomalies,
    anomalyCount,
    hasAnomalies,
    highestSeverity,
  }
}

function reconcileStableAnomalies(
  previousRecords: Map<string, StableAnomalyRecord>,
  nextAnomalies: AdminAnomalyItem[],
) {
  const records = new Map(previousRecords)
  const nextIds = new Set(nextAnomalies.map((item) => item.id))

  nextAnomalies.forEach((item) => {
    const previous = records.get(item.id)
    records.set(item.id, {
      item: {
        ...item,
        detectedAt: previous?.item.detectedAt ?? item.detectedAt,
      },
      missingSamples: 0,
    })
  })

  records.forEach((record, id) => {
    if (nextIds.has(id)) return
    const missingSamples = record.missingSamples + 1
    if (missingSamples >= RECOVERY_SAMPLE_COUNT) {
      records.delete(id)
      return
    }
    records.set(id, { ...record, missingSamples })
  })

  return records
}

function sortAnomalies(items: AdminAnomalyItem[]) {
  return [...items].sort((a, b) => {
    const severityDelta = severityRank[a.severity] - severityRank[b.severity]
    if (severityDelta !== 0) return severityDelta
    return a.detectedAt - b.detectedAt
  })
}
