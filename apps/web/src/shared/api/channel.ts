import { apiDelete, apiGet, apiPost, apiPut } from './client'
import type { ApiResponse } from './types'

export interface Channel {
  id: number
  type: string
  name: string
  api_url: string
  status: number
  created_at: string
  updated_at: string
}

export interface ChannelFormData {
  type: string
  name: string
  api_url: string
}

export type DetectModelsRequest = {
  base_url: string
  api_key: string
}

export type DetectModelsResponse = {
  models: string[]
}

export const channelApi = {
  list: (): Promise<ApiResponse<{ list: Channel[] }>> =>
    apiGet<{ list: Channel[] }>('/admin/channels'),
  create: (data: ChannelFormData): Promise<ApiResponse<{ channel: Channel }>> =>
    apiPost<{ channel: Channel }, ChannelFormData>('/admin/channels', data),
  update: (id: number, data: ChannelFormData): Promise<ApiResponse<{ channel: Channel }>> =>
    apiPut<{ channel: Channel }, ChannelFormData>('/admin/channels/' + id, data),
  delete: (id: number): Promise<ApiResponse<null>> =>
    apiDelete<null>('/admin/channels/' + id),
  detectModels: (data: DetectModelsRequest): Promise<ApiResponse<DetectModelsResponse>> =>
    apiPost<DetectModelsResponse, DetectModelsRequest>('/admin/channels/detect-models', data),
}

export default channelApi
