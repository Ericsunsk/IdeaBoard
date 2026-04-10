import api from './index'
import type { ApiResponse } from '../types'

export interface StyleCard {
  id: number
  name: string
  cover_image: string
  sort_order: number
  is_enabled: boolean
  is_exclusive: boolean
  created_at: string
  updated_at: string
}

export interface CreateStyleCardRequest {
  name: string
  cover_image: string
  keywords: string
  sort_order: number
  is_enabled: boolean
  is_exclusive: boolean
}

export interface UpdateStyleCardRequest {
  name: string
  cover_image: string
  keywords: string
  sort_order: number
  is_enabled: boolean
  is_exclusive: boolean
}

// User-facing API: only returns enabled cards (no keywords exposed)
export const getEnabledCards = (): Promise<ApiResponse<{ list: StyleCard[] }>> =>
  api.get('/user/style-cards') as any

// Admin APIs
export const getStyleCards = (params?: { page?: number; page_size?: number }): Promise<ApiResponse<{ list: StyleCard[]; total: number }>> =>
  api.get('/admin/style-cards', { params }) as any

export const createStyleCard = (data: CreateStyleCardRequest): Promise<ApiResponse<{ card: StyleCard }>> =>
  api.post('/admin/style-cards', data) as any

export const updateStyleCard = (id: number, data: UpdateStyleCardRequest): Promise<ApiResponse<{ card: StyleCard }>> =>
  api.put('/admin/style-cards/' + id, data) as any

export const deleteStyleCard = (id: number): Promise<ApiResponse<null>> =>
  api.delete('/admin/style-cards/' + id) as any

export const styleCardApi = {
  getEnabledCards,
  getStyleCards,
  createStyleCard,
  updateStyleCard,
  deleteStyleCard,
}

export default styleCardApi
