import api from './index'
import type { ApiResponse } from '../types'
import type { Channel, ChannelFormData } from './index'

export type { Channel }

export type DetectModelsRequest = {
  base_url: string
  api_key: string
}

export type DetectModelsResponse = {
  models: string[]
}

export const channelApi = {
  list: (): Promise<ApiResponse<{ list: Channel[] }>> =>
    api.get('/admin/channels') as any,
  create: (data: ChannelFormData): Promise<ApiResponse<{ channel: Channel }>> =>
    api.post('/admin/channels', data) as any,
  update: (id: number, data: ChannelFormData): Promise<ApiResponse<{ channel: Channel }>> =>
    api.put('/admin/channels/' + id, data) as any,
  delete: (id: number): Promise<ApiResponse<null>> =>
    api.delete('/admin/channels/' + id) as any,
  detectModels: (data: DetectModelsRequest): Promise<ApiResponse<DetectModelsResponse>> =>
    api.post('/admin/channels/detect-models', data) as any,
}

export default channelApi
