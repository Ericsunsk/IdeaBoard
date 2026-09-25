import { api } from '../../../../shared/api/client'
import {
  resolveApiData,
  resolveApiOk,
  type ApiResponseBase,
  type ApiResponseWithData,
} from '../../../../shared/api/apiResponse'
import { i18n } from '../../../../shared/i18n'
import type { BoardState } from '../../domain'

// User board sync boundary (issue #809). The generated `boards` client is
// gone but the backend user plane still serves `/api/user/boards/*` with the
// shared envelope, so this boundary transports through the central
// `api.request`. The board document shape is owned by the board domain
// (`BoardState`); this module only moves it across the wire.
//
// TODO(2026-09-25): KEEP hand-rolled — generated `boards` is workspace-scoped
// (`/workspaces/{ws}/boards`, contract envelope) and covers none of these
// legacy `/user/boards/*` flows 1:1 (list/save/remove/join-by-share-token,
// `{code,msg,data}` envelope). Do not force-fit; migration (if any) belongs
// to the backend boards-store owner (sibling A).
export type { ApiResponseBase }
export type BoardStateEnvelope = BoardState

export type UserBoardItem = {
  board_id: string
  name: string
  state: BoardStateEnvelope
  access_role?: 'owner' | 'editor' | 'viewer'
  revision: number
  state_schema_version: number
  created_at: number
  updated_at: number
}

export type SaveUserBoardPayload = {
  name: string
  state: BoardStateEnvelope
  base_revision: number
  created_at: number
  updated_at: number
}

export async function fetchUserBoards(): Promise<UserBoardItem[]> {
  const data = await resolveApiData(
    api.request<ApiResponseWithData<{ list: UserBoardItem[] }>>('/user/boards', {
      method: 'GET',
    }),
    i18n.global.t('workspace.sync.listFailed'),
  )
  return Array.isArray(data.list) ? data.list : []
}

export async function saveUserBoard(boardId: string, payload: SaveUserBoardPayload) {
  return resolveApiData(
    api.request<ApiResponseWithData<{ board: UserBoardItem; new_revision: number }>>(
      `/user/boards/${encodeURIComponent(boardId)}`,
      {
        method: 'PUT',
        body: payload as unknown as Record<string, unknown>,
      },
    ),
    i18n.global.t('workspace.sync.saveFailed'),
  )
}

export async function removeUserBoard(boardId: string): Promise<void> {
  await resolveApiOk(
    api.request<{ code: number; msg: string }>(
      `/user/boards/${encodeURIComponent(boardId)}`,
      { method: 'DELETE' },
    ),
    i18n.global.t('workspace.sync.deleteFailed'),
  )
}

export async function joinUserBoardByShareToken(token: string) {
  return resolveApiData(
    api.request<ApiResponseWithData<{ board: UserBoardItem }>>(
      '/user/boards/share-tokens/join',
      {
        method: 'POST',
        body: { token } as unknown as Record<string, unknown>,
      },
    ),
    i18n.global.t('workspace.joinSharedBoard.joinFailed'),
  )
}
