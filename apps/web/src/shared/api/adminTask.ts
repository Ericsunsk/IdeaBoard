import api from './client'
import type { ApiResponse } from './types'
import type { Task, SubmitTaskData } from './task'

export const adminTaskApi = {
  submit: (data: SubmitTaskData): Promise<ApiResponse<{ task_id: string }>> =>
    api.post('/admin/tasks/submit', data) as any,
  get: (task_id: string): Promise<ApiResponse<{ task: Task }>> =>
    api.get('/admin/tasks/' + task_id) as any,
  list: (params?: { status?: string; page?: number; page_size?: number; user_id?: number; keyword?: string }): Promise<ApiResponse<{ list: Task[]; total: number }>> =>
    api.get('/admin/tasks', { params }) as any,
}
