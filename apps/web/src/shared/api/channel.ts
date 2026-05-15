import {
  createAdminChannel,
  deleteAdminChannel,
  detectAdminChannelModels,
  listAdminChannels,
  updateAdminChannel,
} from './generated-query/admin-channels/admin-channels'
import type {
  Channel as GeneratedChannel,
  ChannelFormData as GeneratedChannelFormData,
  DetectModelsRequest as GeneratedDetectModelsRequest,
  DetectModelsResponse as GeneratedDetectModelsResponse,
} from './generated-query/model'
import type { ApiResponse } from './types'

export type Channel = GeneratedChannel
export type ChannelFormData = GeneratedChannelFormData
export type DetectModelsRequest = GeneratedDetectModelsRequest
export type DetectModelsResponse = GeneratedDetectModelsResponse

export const channelApi = {
  list: (): Promise<ApiResponse<{ list: Channel[] }>> => listAdminChannels(),
  create: (data: ChannelFormData): Promise<ApiResponse<{ channel: Channel }>> =>
    createAdminChannel(data),
  update: (id: number, data: ChannelFormData): Promise<ApiResponse<{ channel: Channel }>> =>
    updateAdminChannel(id, data),
  delete: (id: number): Promise<ApiResponse<null>> => deleteAdminChannel(id),
  detectModels: (data: DetectModelsRequest): Promise<ApiResponse<DetectModelsResponse>> =>
    detectAdminChannelModels(data),
}

export default channelApi
