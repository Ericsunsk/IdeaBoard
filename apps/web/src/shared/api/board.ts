import api from './index'
import type { ApiResponse } from '../types'

export interface UserBoardPayload {
  board_id: string
  name: string
  state: unknown
  created_at?: number
  updated_at?: number
}

export interface UserBoardItem {
  board_id: string
  name: string
  state: unknown
  access_role?: 'owner' | 'editor' | 'viewer'
  created_at: number
  updated_at: number
}

export const boardApi = {
  list: (): Promise<ApiResponse<{ list: UserBoardItem[] }>> =>
    api.get('/user/boards') as any,
  upsert: (boardId: string, payload: Omit<UserBoardPayload, 'board_id'>): Promise<ApiResponse<{ board: UserBoardItem }>> =>
    api.put('/user/boards/' + encodeURIComponent(boardId), payload) as any,
  remove: (boardId: string): Promise<ApiResponse<{ deleted: boolean }>> =>
    api.delete('/user/boards/' + encodeURIComponent(boardId)) as any,
  joinByShareToken: (token: string): Promise<ApiResponse<{ board: UserBoardItem }>> =>
    api.post('/user/boards/share-tokens/join', { token }) as any,
}

export default boardApi
