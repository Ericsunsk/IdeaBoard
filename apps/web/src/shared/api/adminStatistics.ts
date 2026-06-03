import {
  backfillAdminStatistics,
  getAdminStatistics,
} from './generated-query/admin-statistics/admin-statistics'
import type {
  AdminStatistics,
  AdminStatisticsBackfillResult,
  AdminStatisticsBackfillStatus,
  GetAdminStatisticsRange,
} from './generated-query/model'
import type { ApiResponse } from '../types'

export type AdminStatisticsResponse = AdminStatistics
export type AdminStatisticsBackfillState = AdminStatisticsBackfillStatus

export type { AdminStatisticsBackfillResult }

export const adminStatisticsApi = {
  get: (range: GetAdminStatisticsRange): Promise<ApiResponse<AdminStatisticsResponse>> =>
    getAdminStatistics({ range }),
  backfill: (range: GetAdminStatisticsRange): Promise<ApiResponse<AdminStatisticsBackfillResult>> =>
    backfillAdminStatistics({ range }),
}
