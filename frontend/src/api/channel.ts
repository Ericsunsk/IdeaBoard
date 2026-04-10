import api from './index'
import type { ApiResponse } from '../types'

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

export const channelApi = {
  list: (): Promise<ApiResponse<{ list: Channel[] }>> =>
    api.get('/admin/channels') as any,
  create: (data: { type: string; name: string; api_url: string; models: string }): Promise<ApiResponse<{ channel: Channel }>> =>
    api.post('/admin/channels', data) as any,
  update: (id: number, data: { type: string; name: string; api_url: string; models: string }): Promise<ApiResponse<{ channel: Channel }>> =>
    api.put('/admin/channels/' + id, data) as any,
  delete: (id: number): Promise<ApiResponse<null>> =>
    api.delete('/admin/channels/' + id) as any,
}

export default channelApi
