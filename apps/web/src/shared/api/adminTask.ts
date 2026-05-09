import { apiGet, apiPost } from './client'
import type { ApiResponse } from './types'
import type { Task, SubmitTaskData } from './task'

export const adminTaskApi = {
  submit: (data: SubmitTaskData): Promise<ApiResponse<{ task_id: string }>> =>
    apiPost<{ task_id: string }, SubmitTaskData>('/admin/tasks/submit', data),
  get: (task_id: string): Promise<ApiResponse<{ task: Task }>> =>
    apiGet<{ task: Task }>('/admin/tasks/' + task_id),
  list: (params?: { status?: string; page?: number; page_size?: number; user_id?: number; keyword?: string }): Promise<ApiResponse<{ list: Task[]; total: number }>> =>
    apiGet<{ list: Task[]; total: number }>('/admin/tasks', { params }),
}
