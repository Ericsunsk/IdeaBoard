import api from './client'
import type { ApiResponse } from './types'

export const monitorApi = {
  get: (): Promise<ApiResponse<{
    monitor: {
      global_queue_len: number; wait_queue_len: number; running_tasks: number
      account_loads: Record<number, { current_concurrent: number; max_concurrent: number }>
    }
    stats: { pending: number; running: number; completed: number; failed: number }
  }>> =>
    api.get('/admin/monitor') as any,
}
