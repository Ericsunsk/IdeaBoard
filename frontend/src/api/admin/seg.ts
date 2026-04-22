import type { AxiosInstance } from 'axios'
import type { ApiResponse } from '../../types'

export interface SegConfig {
  enabled: boolean
  access_key_id: string
  access_key_secret: string
  region_id: string
}

export interface SegConfigPayload {
  enabled: boolean
  access_key_id: string
  region_id: string
  access_key_secret?: string
}

export function createAdminSegApi(api: AxiosInstance) {
  return {
    getConfig: (): Promise<ApiResponse<{ config: SegConfig }>> =>
      api.get('/admin/seg/config') as unknown as Promise<ApiResponse<{ config: SegConfig }>>,
    saveConfig: (payload: SegConfigPayload): Promise<ApiResponse<null>> =>
      api.post('/admin/seg/config', payload) as unknown as Promise<ApiResponse<null>>,
  }
}
