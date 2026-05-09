import { apiDelete, apiGet, apiPost, apiPut } from './client'
import type { ApiResponse } from './types'

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
    apiGet<{ list: UserBoardItem[] }>('/user/boards'),
  upsert: (boardId: string, payload: Omit<UserBoardPayload, 'board_id'>): Promise<ApiResponse<{ board: UserBoardItem }>> =>
    apiPut<{ board: UserBoardItem }, Omit<UserBoardPayload, 'board_id'>>('/user/boards/' + encodeURIComponent(boardId), payload),
  remove: (boardId: string): Promise<ApiResponse<{ deleted: boolean }>> =>
    apiDelete<{ deleted: boolean }>('/user/boards/' + encodeURIComponent(boardId)),
  joinByShareToken: (token: string): Promise<ApiResponse<{ board: UserBoardItem }>> =>
    apiPost<{ board: UserBoardItem }, { token: string }>('/user/boards/share-tokens/join', { token }),
}

export default boardApi
