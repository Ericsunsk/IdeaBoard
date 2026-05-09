import { apiGet } from './client'
import type { ApiResponse } from './types'

export interface MonitorResponse {
  monitor: {
    global_queue_len: number; wait_queue_len: number; running_tasks: number
    account_loads: Record<number, { current_concurrent: number; max_concurrent: number }>
  }
  stats: { pending: number; running: number; completed: number; failed: number }
}

export const monitorApi = {
  get: (): Promise<ApiResponse<MonitorResponse>> =>
    apiGet<MonitorResponse>('/admin/monitor'),
}
