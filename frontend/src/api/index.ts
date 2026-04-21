import axios from 'axios'
import type {
  LoginRequest,
  RegisterRequest,
  LoginResponse,
  UpdatePasswordRequest,
  CreateUserRequest,
  UserListResponse,
  User,
  ApiResponse,
} from '../types'

type ApiClientError = Error & {
  code?: number
  msg?: string
  status?: number
  data?: unknown
}

const api = axios.create({
  baseURL: '/api',
  timeout: 10000,
  headers: {
    'Content-Type': 'application/json'
  }
})

function createApiError(payload?: any, status?: number): ApiClientError {
  const message = payload?.msg || payload?.message || '请求失败'
  const error = new Error(message) as ApiClientError
  error.code = payload?.code ?? status
  error.msg = payload?.msg || message
  error.status = status
  error.data = payload?.data
  return error
}

function clearStoredAuth() {
  localStorage.removeItem('token')
  localStorage.removeItem('user')
}

function isAuthRequest(requestUrl: string) {
  return requestUrl.includes('/auth/')
}

function isAdminApiRequest(requestUrl: string) {
  return requestUrl.includes('/admin/')
}

function isAdminPage() {
  return window.location.pathname.startsWith('/admin')
}

function buildLoginRedirectPath() {
  const currentPath = window.location.pathname + window.location.search
  const loginPath = isAdminPage() ? '/admin/login' : '/user/login'
  return loginPath + '?redirect=' + encodeURIComponent(currentPath)
}

function handleAuthExpired(status: number | undefined, payload: any, requestUrl: string) {
  const code = payload?.code ?? status

  if (code === 403 && isAdminApiRequest(requestUrl) && isAdminPage()) {
    window.location.href = '/user/home'
    return
  }

  if (code !== 401 || isAuthRequest(requestUrl)) {
    return
  }

  clearStoredAuth()
  window.location.href = buildLoginRedirectPath()
}

api.interceptors.request.use(
  (config) => {
    const token = localStorage.getItem('token')
    if (token) {
      config.headers.Authorization = 'Bearer ' + token
    }
    return config
  },
  (error) => {
    return Promise.reject(error)
  }
)

api.interceptors.response.use(
  (response) => {
    const data = response.data
    if (data && typeof data.code === 'number' && data.code !== 0) {
      const requestUrl = response.config?.url || ''
      handleAuthExpired(response.status, data, requestUrl)
      return Promise.reject(createApiError(data, response.status))
    }
    return data
  },
  (error) => {
    if (error.response) {
      const requestUrl = error.config?.url || ''
      handleAuthExpired(error.response.status, error.response.data, requestUrl)
      return Promise.reject(createApiError(error.response.data, error.response.status))
    }
    return Promise.reject(createApiError(error))
  }
)

export const authApi = {
  login: (data: LoginRequest): Promise<ApiResponse<LoginResponse>> =>
    api.post('/auth/login', data) as any,
  register: (data: RegisterRequest): Promise<ApiResponse<{ user: User }>> =>
    api.post('/auth/register', data) as any,
}

export const userApi = {
  getInfo: (): Promise<ApiResponse<{ user: User }>> =>
    api.get('/user/info') as any,
  updatePassword: (data: UpdatePasswordRequest): Promise<ApiResponse<null>> =>
    api.post('/user/password', data) as any,
  logout: (): Promise<ApiResponse<null>> =>
    api.post('/user/logout') as any,
}

export const adminApi = {
  getUsers: (params?: { keyword?: string; page?: number; page_size?: number }): Promise<ApiResponse<UserListResponse>> =>
    api.get('/admin/users', { params }) as any,
  createUser: (data: CreateUserRequest): Promise<ApiResponse<{ user: User }>> =>
    api.post('/admin/users', data) as any,
  updateUserStatus: (id: number, status: number): Promise<ApiResponse<null>> =>
    api.put('/admin/users/' + id + '/status', { status }) as any,
  updateUserPassword: (id: number, new_password: string): Promise<ApiResponse<null>> =>
    api.put('/admin/users/' + id + '/password', { new_password }) as any,
  deleteUser: (id: number): Promise<ApiResponse<null>> =>
    api.delete('/admin/users/' + id) as any,
}

// ============ 风格卡片 API ============

export interface StyleCard {
  id: number
  name: string
  cover_image: string
  keywords?: string
  sort_order: number
  is_enabled: boolean
  is_exclusive: boolean
  created_at: string
  updated_at: string
}

export interface StyleCardFormData {
  name: string
  cover_image: string
  keywords: string
  sort_order: number
  is_enabled: boolean
  is_exclusive: boolean
}

export const styleCardApi = {
  getEnabled: (): Promise<ApiResponse<{ list: StyleCard[] }>> =>
    api.get('/user/style-cards') as any,
  getList: (params?: { page?: number; page_size?: number }): Promise<ApiResponse<{ list: StyleCard[]; total: number }>> =>
    api.get('/admin/style-cards', { params }) as any,
  create: (data: StyleCardFormData): Promise<ApiResponse<{ card: StyleCard }>> =>
    api.post('/admin/style-cards', data) as any,
  update: (id: number, data: StyleCardFormData): Promise<ApiResponse<{ card: StyleCard }>> =>
    api.put('/admin/style-cards/' + id, data) as any,
  delete: (id: number): Promise<ApiResponse<null>> =>
    api.delete('/admin/style-cards/' + id) as any,
}

// ============ 账号管理 API ============

export interface ApiAccount {
  id: number
  name: string
  channel_id: number
  platform: string
  api_type: string
  model_set: string
  proxy_url: string
  max_concurrent: number
  timeout: number
  extra_config: string
  status: number
  fail_count: number
  weight: number
  created_at: string
  updated_at: string
}

export interface ApiAccountWithLoad extends ApiAccount {
  current_concurrent: number
  wait_queue_count: number
  running_status: string
}

export interface ApiAccountFormData {
  name: string
  channel_id: number
  platform: string
  api_type: string
  model_set: string
  api_key?: string
  proxy_url: string
  max_concurrent: number
  timeout: number
  extra_config: string
  weight: number
  status?: number
}

export const accountApi = {
  getList: (params?: { keyword?: string; platform?: string; page?: number; page_size?: number }): Promise<ApiResponse<{ list: ApiAccountWithLoad[]; total: number }>> =>
    api.get('/admin/accounts', { params }) as any,
  create: (data: ApiAccountFormData): Promise<ApiResponse<{ account: ApiAccount }>> =>
    api.post('/admin/accounts', data) as any,
  update: (id: number, data: ApiAccountFormData): Promise<ApiResponse<null>> =>
    api.put('/admin/accounts/' + id, data) as any,
  delete: (id: number): Promise<ApiResponse<null>> =>
    api.delete('/admin/accounts/' + id) as any,
  start: (id: number): Promise<ApiResponse<null>> =>
    api.post('/admin/accounts/' + id + '/start') as any,
  stop: (id: number): Promise<ApiResponse<null>> =>
    api.post('/admin/accounts/' + id + '/stop') as any,
}

// ============ 任务 API ============

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
  card_ids?: number[]
  feature?: string
  image_urls?: string[]
  params?: Record<string, any>
}

export const segApi = {
  segment: (data: { image_url: string; seg_type: string; return_form?: string }): Promise<ApiResponse<{ result_url: string }>> =>
    api.post('/user/segment', data) as any,
  save: (data: { result_url: string; seg_type: string; feature?: string }): Promise<ApiResponse<{ image_url: string; task_id: string }>> =>
    api.post('/user/segment/save', data) as any,
}

// ============ 线稿模式 API ============

export interface LineDraftMode {
  id: number
  name: string
  cover_image: string
  prompt: string
  sort_order: number
  is_enabled: boolean
  created_at: string
  updated_at: string
}

export interface LineDraftModeFormData {
  name: string
  cover_image: string
  prompt: string
  sort_order: number
  is_enabled: boolean
}

export const lineDraftApi = {
  listModes: (): Promise<ApiResponse<{ list: LineDraftMode[] }>> =>
    api.get('/user/line-draft-modes') as any,
  adminList: (): Promise<ApiResponse<{ list: LineDraftMode[] }>> =>
    api.get('/admin/line-draft-modes') as any,
  adminCreate: (data: LineDraftModeFormData): Promise<ApiResponse<{ mode: LineDraftMode }>> =>
    api.post('/admin/line-draft-modes', data) as any,
  adminUpdate: (id: number, data: LineDraftModeFormData): Promise<ApiResponse<null>> =>
    api.put('/admin/line-draft-modes/' + id, data) as any,
  adminDelete: (id: number): Promise<ApiResponse<null>> =>
    api.delete('/admin/line-draft-modes/' + id) as any,
}

export const featurePromptApi = {
  adminList: (): Promise<ApiResponse<{ prompts: Record<string, string> }>> =>
    api.get('/admin/feature-prompts') as any,
  adminGet: (slug: string): Promise<ApiResponse<{ slug: string; prompt: string }>> =>
    api.get('/admin/feature-prompts/' + slug) as any,
  adminSet: (slug: string, prompt: string): Promise<ApiResponse<{ slug: string; prompt: string }>> =>
    api.put('/admin/feature-prompts/' + slug, { prompt }) as any,
}

// ============ 人脸模板 API ============

export interface FaceTemplate {
  id: number
  name: string
  image_url: string
  category: 'male' | 'female'
  sort_order: number
  is_enabled: boolean
  created_at: string
  updated_at: string
}

export interface FaceTemplateFormData {
  name: string
  image_url: string
  category: string
  sort_order: number
  is_enabled: boolean
}

export const faceTemplateApi = {
  listEnabled: (): Promise<ApiResponse<{ list: FaceTemplate[] }>> =>
    api.get('/user/face-templates') as any,
  adminList: (params?: { page?: number; page_size?: number }): Promise<ApiResponse<{ list: FaceTemplate[]; total: number }>> =>
    api.get('/admin/face-templates', { params }) as any,
  adminCreate: (data: FaceTemplateFormData): Promise<ApiResponse<{ face: FaceTemplate }>> =>
    api.post('/admin/face-templates', data) as any,
  adminUpdate: (id: number, data: FaceTemplateFormData): Promise<ApiResponse<null>> =>
    api.put('/admin/face-templates/' + id, data) as any,
  adminDelete: (id: number): Promise<ApiResponse<null>> =>
    api.delete('/admin/face-templates/' + id) as any,
}

// ============ 背景模板 API ============

export interface BgTemplate {
  id: number
  name: string
  image_url: string
  category: string
  sort_order: number
  is_enabled: boolean
  created_at: string
  updated_at: string
}

export interface BgTemplateFormData {
  name: string
  image_url: string
  category: string
  sort_order: number
  is_enabled: boolean
}

export const bgTemplateApi = {
  listEnabled: (): Promise<ApiResponse<{ list: BgTemplate[] }>> =>
    api.get('/user/bg-templates') as any,
  adminList: (params?: { page?: number; page_size?: number }): Promise<ApiResponse<{ list: BgTemplate[]; total: number }>> =>
    api.get('/admin/bg-templates', { params }) as any,
  adminCreate: (data: BgTemplateFormData): Promise<ApiResponse<{ bg: BgTemplate }>> =>
    api.post('/admin/bg-templates', data) as any,
  adminUpdate: (id: number, data: BgTemplateFormData): Promise<ApiResponse<null>> =>
    api.put('/admin/bg-templates/' + id, data) as any,
  adminDelete: (id: number): Promise<ApiResponse<null>> =>
    api.delete('/admin/bg-templates/' + id) as any,
}

// ============ 图案风格 API ============

export interface PatternStyle {
  id: number
  name: string
  image_url: string
  category: string
  sort_order: number
  is_enabled: boolean
  created_at: string
  updated_at: string
}

export interface PatternStyleFormData {
  name: string
  image_url: string
  category: string
  sort_order: number
  is_enabled: boolean
}

export const patternStyleApi = {
  listEnabled: (category?: string): Promise<ApiResponse<{ list: PatternStyle[] }>> =>
    api.get('/user/pattern-styles', { params: category ? { category } : {} }) as any,
  adminList: (params?: { page?: number; page_size?: number }): Promise<ApiResponse<{ list: PatternStyle[]; total: number }>> =>
    api.get('/admin/pattern-styles', { params }) as any,
  adminCreate: (data: PatternStyleFormData): Promise<ApiResponse<{ pattern_style: PatternStyle }>> =>
    api.post('/admin/pattern-styles', data) as any,
  adminUpdate: (id: number, data: PatternStyleFormData): Promise<ApiResponse<null>> =>
    api.put('/admin/pattern-styles/' + id, data) as any,
  adminDelete: (id: number): Promise<ApiResponse<null>> =>
    api.delete('/admin/pattern-styles/' + id) as any,
}

// ============ 渠道商 API ============

export interface Channel {
  id: number
  type: string
  name: string
  api_url: string
  models: string
  status: number
  created_at: string
  updated_at: string
}

export interface ChannelFormData {
  type: string
  name: string
  api_url: string
  models: string
}

export const upscaleApi = {
  upscale: (data: { image_url: string; mode: string; upscale_factor: number; output_format: string; output_quality: number; feature?: string }): Promise<ApiResponse<{ image_url: string; task_id: string }>> =>
    api.post('/user/upscale', data, { timeout: 120000 }) as any,
}

export const vectorizeApi = {
  vectorize: (data: {
    image_url: string; threshold: number; turd_size: number;
    alpha_max: number; opt_tolerance: number; turn_policy: string;
    invert: boolean; color_count: number;
  }): Promise<ApiResponse<{ svg_url: string; task_id: string }>> =>
    api.post('/user/vectorize', data, { timeout: 120000 }) as any,
  checkPotrace: (): Promise<ApiResponse<{ version: string }>> =>
    api.get('/user/vectorize/check') as any,
}

export const uploadApi = {
  upload: (file: File): Promise<ApiResponse<{ url: string }>> => {
    const form = new FormData()
    form.append('file', file)
    return api.post('/user/upload', form, { headers: { 'Content-Type': 'multipart/form-data' } }) as any
  },
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

// ============ 监控 API ============

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

// ============ 七牛云 Kodo 配置 API ============

export interface KodoConfig {
  enabled: boolean
  access_key: string
  secret_key: string
  bucket_name: string
  upload_domain: string
  access_domain: string
  thumbnail_mode: boolean
  direct_fetch: boolean
}

export const kodoApi = {
  getConfig: (): Promise<ApiResponse<{ config: KodoConfig }>> =>
    api.get('/admin/kodo/config') as any,
  saveConfig: (data: KodoConfig): Promise<ApiResponse<{ message: string }>> =>
    api.post('/admin/kodo/config', data) as any,
  testConnection: (): Promise<ApiResponse<{ message: string }>> =>
    api.post('/admin/kodo/test') as any,
  getBucketInfo: (): Promise<ApiResponse<{ info: { domains?: string[] } }>> =>
    api.get('/admin/kodo/bucket-info') as any,
}

export default api
