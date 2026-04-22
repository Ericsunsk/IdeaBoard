import type { AxiosInstance } from 'axios'
import type { ApiResponse } from '../../types'

export interface FaceTemplate {
  id: number
  name: string
  image_url: string
  category: 'male' | 'female'
  sort_order: number
  is_enabled: boolean
  created_at: string
  updated_at: string
}

export interface FaceTemplateFormData {
  name: string
  image_url: string
  category: string
  sort_order: number
  is_enabled: boolean
}

export interface BgTemplate {
  id: number
  name: string
  image_url: string
  category: string
  sort_order: number
  is_enabled: boolean
  created_at: string
  updated_at: string
}

export interface BgTemplateFormData {
  name: string
  image_url: string
  category: string
  sort_order: number
  is_enabled: boolean
}

export interface PatternStyle {
  id: number
  name: string
  image_url: string
  category: string
  sort_order: number
  is_enabled: boolean
  created_at: string
  updated_at: string
}

export interface PatternStyleFormData {
  name: string
  image_url: string
  category: string
  sort_order: number
  is_enabled: boolean
}

export function createAdminAssetsApi(api: AxiosInstance) {
  const faceTemplateApi = {
    listEnabled: (): Promise<ApiResponse<{ list: FaceTemplate[] }>> =>
      api.get('/user/face-templates') as unknown as Promise<ApiResponse<{ list: FaceTemplate[] }>>,
    adminList: (params?: { page?: number; page_size?: number }): Promise<ApiResponse<{ list: FaceTemplate[]; total: number }>> =>
      api.get('/admin/face-templates', { params }) as unknown as Promise<ApiResponse<{ list: FaceTemplate[]; total: number }>>,
    adminCreate: (data: FaceTemplateFormData): Promise<ApiResponse<{ face: FaceTemplate }>> =>
      api.post('/admin/face-templates', data) as unknown as Promise<ApiResponse<{ face: FaceTemplate }>>,
    adminUpdate: (id: number, data: FaceTemplateFormData): Promise<ApiResponse<null>> =>
      api.put('/admin/face-templates/' + id, data) as unknown as Promise<ApiResponse<null>>,
    adminDelete: (id: number): Promise<ApiResponse<null>> =>
      api.delete('/admin/face-templates/' + id) as unknown as Promise<ApiResponse<null>>,
  }

  const bgTemplateApi = {
    listEnabled: (): Promise<ApiResponse<{ list: BgTemplate[] }>> =>
      api.get('/user/bg-templates') as unknown as Promise<ApiResponse<{ list: BgTemplate[] }>>,
    adminList: (params?: { page?: number; page_size?: number }): Promise<ApiResponse<{ list: BgTemplate[]; total: number }>> =>
      api.get('/admin/bg-templates', { params }) as unknown as Promise<ApiResponse<{ list: BgTemplate[]; total: number }>>,
    adminCreate: (data: BgTemplateFormData): Promise<ApiResponse<{ bg: BgTemplate }>> =>
      api.post('/admin/bg-templates', data) as unknown as Promise<ApiResponse<{ bg: BgTemplate }>>,
    adminUpdate: (id: number, data: BgTemplateFormData): Promise<ApiResponse<null>> =>
      api.put('/admin/bg-templates/' + id, data) as unknown as Promise<ApiResponse<null>>,
    adminDelete: (id: number): Promise<ApiResponse<null>> =>
      api.delete('/admin/bg-templates/' + id) as unknown as Promise<ApiResponse<null>>,
  }

  const patternStyleApi = {
    listEnabled: (category?: string): Promise<ApiResponse<{ list: PatternStyle[] }>> =>
      api.get('/user/pattern-styles', { params: category ? { category } : {} }) as unknown as Promise<ApiResponse<{ list: PatternStyle[] }>>,
    adminList: (params?: { page?: number; page_size?: number }): Promise<ApiResponse<{ list: PatternStyle[]; total: number }>> =>
      api.get('/admin/pattern-styles', { params }) as unknown as Promise<ApiResponse<{ list: PatternStyle[]; total: number }>>,
    adminCreate: (data: PatternStyleFormData): Promise<ApiResponse<{ pattern_style: PatternStyle }>> =>
      api.post('/admin/pattern-styles', data) as unknown as Promise<ApiResponse<{ pattern_style: PatternStyle }>>,
    adminUpdate: (id: number, data: PatternStyleFormData): Promise<ApiResponse<null>> =>
      api.put('/admin/pattern-styles/' + id, data) as unknown as Promise<ApiResponse<null>>,
    adminDelete: (id: number): Promise<ApiResponse<null>> =>
      api.delete('/admin/pattern-styles/' + id) as unknown as Promise<ApiResponse<null>>,
  }

  return {
    faceTemplateApi,
    bgTemplateApi,
    patternStyleApi,
  }
}
