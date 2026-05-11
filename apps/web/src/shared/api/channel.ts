import { apiPost } from './client'
import { getAdminChannels } from './generated/admin-channels/admin-channels'
import type {
  Channel as GeneratedChannel,
  ChannelFormData as GeneratedChannelFormData,
} from './generated/model'
import type { ApiResponse } from './types'

export type Channel = GeneratedChannel
export type ChannelFormData = GeneratedChannelFormData

export type DetectModelsRequest = {
  base_url: string
  api_key: string
}

export type DetectModelsResponse = {
  models: string[]
}

const adminChannelsApi = getAdminChannels()

export const channelApi = {
  list: (): Promise<ApiResponse<{ list: Channel[] }>> => adminChannelsApi.listAdminChannels(),
  create: (data: ChannelFormData): Promise<ApiResponse<{ channel: Channel }>> =>
    adminChannelsApi.createAdminChannel(data),
  update: (id: number, data: ChannelFormData): Promise<ApiResponse<{ channel: Channel }>> =>
    adminChannelsApi.updateAdminChannel(id, data),
  delete: (id: number): Promise<ApiResponse<null>> => adminChannelsApi.deleteAdminChannel(id),
  detectModels: (data: DetectModelsRequest): Promise<ApiResponse<DetectModelsResponse>> =>
    apiPost<DetectModelsResponse, DetectModelsRequest>('/admin/channels/detect-models', data),
}

export default channelApi
