import api from './index'
import type { Task as BaseTask, SubmitTaskData } from './index'
import type { ApiResponse } from '../types'

export type Task = BaseTask
export type SubmitTaskRequest = SubmitTaskData

export interface TaskListResponse {
  list: Task[]
  total: number
}

export interface ApiAccount {
  id: number
  name: string
  channel_id: number
  platform: string
  api_type: string
  api_key: string
  proxy_url: string
  model_set: string
  max_concurrent: number
  current_concurrent: number
  fail_count: number
  weight: number
  timeout: number
  extra_config: string
  status: number
  created_at: string
  updated_at: string
}

export interface CreateAccountRequest {
  name: string
  platform: string
  api_type: string
  api_key: string
  model_set?: string
  proxy_url?: string
  max_concurrent?: number
  timeout?: number
  weight?: number
  extra_config?: string
}

export interface UpdateAccountRequest {
  name?: string
  platform?: string
  api_type?: string
  api_key?: string
  model_set?: string
  proxy_url?: string
  max_concurrent?: number
  timeout?: number
  weight?: number
  extra_config?: string
  status?: number
}

export interface AccountListResponse {
  list: ApiAccount[]
  total: number
}

export interface MonitorResponse {
  monitor: {
    global_queue_len: number
    wait_queue_len: number
    running_tasks: number
    account_loads: Record<number, { current_concurrent: number; max_concurrent: number }>
  }
  stats: {
    pending: number
    running: number
    completed: number
    failed: number
  }
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
}

// User task APIs - backend uses /api/user/drawing/...
export const submitTask = (data: SubmitTaskRequest): Promise<ApiResponse<{ task_id: string }>> =>
  api.post('/user/drawing/submit', data) as any

export const getTask = (task_id: string): Promise<ApiResponse<{ task: Task }>> =>
  api.get('/user/drawing/' + task_id) as any

export const listTasks = (params?: { page?: number; page_size?: number }): Promise<ApiResponse<TaskListResponse>> =>
  api.get('/user/drawing', { params }) as any

// Admin task APIs - backend uses /api/admin/tasks/...
export const submitAdminTask = (data: SubmitTaskRequest): Promise<ApiResponse<{ task_id: string }>> =>
  api.post('/admin/tasks/submit', data) as any

export const getAdminTask = (task_id: string): Promise<ApiResponse<{ task: Task }>> =>
  api.get('/admin/tasks/' + task_id) as any

export const listAdminTasks = (params?: { page?: number; page_size?: number; status?: string; user_id?: number; keyword?: string }): Promise<ApiResponse<TaskListResponse>> =>
  api.get('/admin/tasks', { params }) as any

// Account management APIs
export const createAccount = (data: CreateAccountRequest): Promise<ApiResponse<{ account: ApiAccount }>> =>
  api.post('/admin/accounts', data) as any

export const updateAccount = (id: number, data: UpdateAccountRequest): Promise<ApiResponse<null>> =>
  api.put('/admin/accounts/' + id, data) as any

export const deleteAccount = (id: number): Promise<ApiResponse<null>> =>
  api.delete('/admin/accounts/' + id) as any

export const listAccounts = (params?: { keyword?: string; platform?: string; page?: number; page_size?: number }): Promise<ApiResponse<AccountListResponse>> =>
  api.get('/admin/accounts', { params }) as any

export const startAccount = (id: number): Promise<ApiResponse<null>> =>
  api.post('/admin/accounts/' + id + '/start') as any

export const stopAccount = (id: number): Promise<ApiResponse<null>> =>
  api.post('/admin/accounts/' + id + '/stop') as any

export const getMonitor = (): Promise<ApiResponse<MonitorResponse>> =>
  api.get('/admin/monitor') as any

export const getAdminStatistics = (range: '7d' | '30d' | '1y'): Promise<ApiResponse<AdminStatisticsResponse>> =>
  api.get('/admin/statistics', { params: { range } }) as any

export const accountApi = {
  submitTask,
  getTask,
  listTasks,
  submitAdminTask,
  getAdminTask,
  listAdminTasks,
  createAccount,
  updateAccount,
  deleteAccount,
  listAccounts,
  startAccount,
  stopAccount,
  getMonitor,
  getAdminStatistics,
}

export default accountApi
