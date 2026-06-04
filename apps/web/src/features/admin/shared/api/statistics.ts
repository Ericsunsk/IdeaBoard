import type {
  AdminStatistics,
  AdminStatisticsBackfillResult,
  AdminStatisticsBackfillStatus,
} from '../../../../shared/api/generated-query/model'

export type AdminStatisticsResponse = AdminStatistics
export type AdminStatisticsBackfillState = AdminStatisticsBackfillStatus

export type { AdminStatisticsBackfillResult }
