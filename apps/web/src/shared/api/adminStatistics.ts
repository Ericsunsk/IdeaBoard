import { apiGet, apiPost } from './client'
import type { ApiResponse } from './types'

export interface AdminStatisticsBackfillState {
  range_start: string
  range_end: string
  missing_days: number
  backfilled_days: number
}

export interface AdminStatisticsBackfillResult {
  range: string
  range_start: string
  range_end: string
  scanned_days: number
  backfilled_days: number
}

export interface AdminStatisticsResponse {
  summary: {
    total_calls: number
    today_calls: number
    today_success: number
    today_failed: number
    yesterday_calls: number
    yesterday_success: number
    yesterday_failed: number
    this_month_calls: number
    pending: number
    running: number
    completed: number
    failed: number
  }
  trend: Array<{
    date: string
    label: string
    total: number
    segments: Array<{ label: string; value: number }>
  }>
  model_ranking: Array<{ name: string; value: number; percent: number }>
  user_rank: Array<{ name: string; value: number }>
  latency: Array<{ name: string; avg_seconds: number }>
  health_cells: Array<{ status: number; success: number; fail: number; total: number }>
  backfill_status?: AdminStatisticsBackfillState
}

export const adminStatisticsApi = {
  get: (range: '7d' | '30d' | '1y'): Promise<ApiResponse<AdminStatisticsResponse>> =>
    apiGet<AdminStatisticsResponse>('/admin/statistics', { params: { range } }),
  backfill: (range: '7d' | '30d' | '1y'): Promise<ApiResponse<AdminStatisticsBackfillResult>> =>
    apiPost<AdminStatisticsBackfillResult, { range: '7d' | '30d' | '1y' }>('/admin/statistics/backfill', { range }),
}
