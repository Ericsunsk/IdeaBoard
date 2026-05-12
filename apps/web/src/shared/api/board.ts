import {
  deleteUserBoard,
  joinBoardByShareToken,
  listUserBoards,
  upsertUserBoard,
} from './generated-query/boards/boards'
import type { UserBoardUpsertRequestState } from './generated-query/model'
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
    listUserBoards(),
  upsert: (
    boardId: string,
    payload: Omit<UserBoardPayload, 'board_id'>,
  ): Promise<ApiResponse<{ board: UserBoardItem }>> =>
    upsertUserBoard(boardId, {
      ...payload,
      state: (payload.state ?? null) as UserBoardUpsertRequestState,
    }),
  remove: (boardId: string): Promise<ApiResponse<{ deleted: boolean }>> =>
    deleteUserBoard(boardId),
  joinByShareToken: (token: string): Promise<ApiResponse<{ board: UserBoardItem }>> =>
    joinBoardByShareToken({ token }),
}

export default boardApi
