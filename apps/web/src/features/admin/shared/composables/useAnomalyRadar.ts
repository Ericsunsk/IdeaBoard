import { computed, ref, watch, type Ref } from 'vue'
import { useGetAdminMonitor } from '../../../../shared/api/generated-query/admin-monitor/admin-monitor'
import type { MonitorResponse } from '../../../../shared/api/generated-query/model'
import { createApiDataSelector } from '../../../../shared/api/apiResponse'
import { GetAdminStatisticsRange, useAdminStatisticsResource } from '../../statistics/resource'
import {
  buildAdminAnomalyItems,
  type AdminAnomalyItem,
  type AdminAnomalySeverity,
} from '../lib/anomalyRules'
import { useQueryPolling } from './useQueryPolling'

type UseAdminAnomalyRadarOptions = {
  enabled?: Ref<boolean>
}

type StableAnomalyRecord = {
  item: AdminAnomalyItem
  missingSamples: number
}

const RECOVERY_SAMPLE_COUNT = 3
const selectAdminMonitor = createApiDataSelector<MonitorResponse>('加载监控数据失败')
const severityRank: Record<AdminAnomalySeverity, number> = {
  critical: 0,
  warning: 1,
  info: 2,
}

export function useAnomalyRadar(options: UseAdminAnomalyRadarOptions = {}) {
  const isEnabled = computed(() => options.enabled?.value ?? true)
  const polling = useQueryPolling({
    visibleInterval: 15_000,
    hiddenInterval: 60_000,
    isEnabled: () => isEnabled.value,
  })

  const monitorQuery = useGetAdminMonitor<MonitorResponse>({
    query: {
      enabled: isEnabled,
      refetchInterval: polling.refetchInterval,
      refetchIntervalInBackground: false,
      refetchOnWindowFocus: false,
      select: selectAdminMonitor,
    },
  })
  const statisticsResource = useAdminStatisticsResource(
    computed(() => ({ range: GetAdminStatisticsRange['1d'] })),
    {
      enabled: isEnabled,
      refetchInterval: polling.refetchInterval,
    },
  )

  const monitorData = computed(() => monitorQuery.data.value?.monitor || null)
  const monitorStats = computed(() => monitorQuery.data.value?.stats || null)
  const statistics = statisticsResource.statistics
  const stableRecords = ref(new Map<string, StableAnomalyRecord>())
  const anomalies = ref<AdminAnomalyItem[]>([])

  const rawAnomalies = computed(() => {
    if (!isEnabled.value) return []
    return buildAdminAnomalyItems({
      monitorData: monitorData.value,
      monitorStats: monitorStats.value,
      statistics: statistics.value,
      isMonitorUnavailable: monitorQuery.isError.value,
      isStatisticsUnavailable: statisticsResource.isError.value,
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
