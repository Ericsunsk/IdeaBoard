import { apiPost } from './client'
import { getAdminStatistics } from './generated-query/admin-statistics/admin-statistics'
import type {
  AdminStatistics,
  AdminStatisticsBackfillStatus,
  GetAdminStatisticsRange,
} from './generated-query/model'
import type { ApiResponse } from '../types'

export type AdminStatisticsResponse = AdminStatistics
export type AdminStatisticsBackfillState = AdminStatisticsBackfillStatus

export interface AdminStatisticsBackfillResult {
  range: string
  range_start: string
  range_end: string
  scanned_days: number
  backfilled_days: number
}

export const adminStatisticsApi = {
  get: (range: GetAdminStatisticsRange): Promise<ApiResponse<AdminStatisticsResponse>> =>
    getAdminStatistics({ range }),
  backfill: (range: GetAdminStatisticsRange): Promise<ApiResponse<AdminStatisticsBackfillResult>> =>
    apiPost<AdminStatisticsBackfillResult>('/admin/statistics/backfill', {
      body: { range },
    }),
}
