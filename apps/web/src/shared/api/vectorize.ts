import api from './client'
import type { ApiResponse } from './types'

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
