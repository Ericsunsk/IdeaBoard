import { apiGet, apiPut } from './client'
import type { ApiResponse } from './types'

export const featurePromptApi = {
  adminList: (): Promise<ApiResponse<{ prompts: Record<string, string> }>> =>
    apiGet<{ prompts: Record<string, string> }>('/admin/feature-prompts'),
  adminGet: (slug: string): Promise<ApiResponse<{ slug: string; prompt: string }>> =>
    apiGet<{ slug: string; prompt: string }>('/admin/feature-prompts/' + slug),
  adminSet: (slug: string, prompt: string): Promise<ApiResponse<{ slug: string; prompt: string }>> =>
    apiPut<{ slug: string; prompt: string }, { prompt: string }>('/admin/feature-prompts/' + slug, { prompt }),
}
