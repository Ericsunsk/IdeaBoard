import api from './client'
import type { ApiResponse } from './types'

export interface Task {
  id: number
  task_id: string
  user_id: number
  channel_id?: number
  account_id?: number
  prompt: string
  params?: string
  status: string
  retry_count?: number
  error_msg?: string
  result?: string
  created_at: string
  updated_at: string
  completed_at?: string | null
}

export interface SubmitTaskData {
  prompt: string
  feature?: string
  image_urls?: string[]
  params?: Record<string, any>
}

export const taskApi = {
  submit: (data: SubmitTaskData): Promise<ApiResponse<{ task_id: string }>> =>
    api.post('/user/drawing/submit', data) as any,
  get: (task_id: string): Promise<ApiResponse<{ task: Task }>> =>
    api.get('/user/drawing/' + task_id) as any,
  list: (params?: { page?: number; page_size?: number }): Promise<ApiResponse<{ list: Task[]; total: number }>> =>
    api.get('/user/drawing', { params }) as any,
  delete: (task_id: string): Promise<ApiResponse<{ message: string }>> =>
    api.delete('/user/drawing/' + task_id) as any,
}
