import api from './index'
import type { ApiResponse } from '../types'
import type { Channel, ChannelFormData } from './index'

export type { Channel }

export const channelApi = {
  list: (): Promise<ApiResponse<{ list: Channel[] }>> =>
    api.get('/admin/channels') as any,
  create: (data: ChannelFormData): Promise<ApiResponse<{ channel: Channel }>> =>
    api.post('/admin/channels', data) as any,
  update: (id: number, data: ChannelFormData): Promise<ApiResponse<{ channel: Channel }>> =>
    api.put('/admin/channels/' + id, data) as any,
  delete: (id: number): Promise<ApiResponse<null>> =>
    api.delete('/admin/channels/' + id) as any,
}

export default channelApi
