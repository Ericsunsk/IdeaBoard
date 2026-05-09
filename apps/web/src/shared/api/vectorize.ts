import { apiGet, apiPost } from './client'
import type { ApiResponse } from './types'

export const vectorizeApi = {
  vectorize: (data: {
    image_url: string; threshold: number; turd_size: number;
    alpha_max: number; opt_tolerance: number; turn_policy: string;
    invert: boolean; color_count: number;
  }): Promise<ApiResponse<{ svg_url: string; task_id: string }>> =>
    apiPost<{ svg_url: string; task_id: string }, {
      image_url: string; threshold: number; turd_size: number;
      alpha_max: number; opt_tolerance: number; turn_policy: string;
      invert: boolean; color_count: number;
    }>('/user/vectorize', data, { timeout: 120000 }),
  checkPotrace: (): Promise<ApiResponse<{ version: string }>> =>
    apiGet<{ version: string }>('/user/vectorize/check'),
}
