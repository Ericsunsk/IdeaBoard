import api from './client'
import type { ApiResponse } from './types'

export const featurePromptApi = {
  adminList: (): Promise<ApiResponse<{ prompts: Record<string, string> }>> =>
    api.get('/admin/feature-prompts') as any,
  adminGet: (slug: string): Promise<ApiResponse<{ slug: string; prompt: string }>> =>
    api.get('/admin/feature-prompts/' + slug) as any,
  adminSet: (slug: string, prompt: string): Promise<ApiResponse<{ slug: string; prompt: string }>> =>
    api.put('/admin/feature-prompts/' + slug, { prompt }) as any,
}
