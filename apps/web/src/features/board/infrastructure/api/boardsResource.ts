import {
  deleteUserBoard,
  joinBoardByShareToken,
  listUserBoards,
  upsertUserBoard,
} from '../../../../shared/api/generated-query/boards/boards'
import type {
  ApiResponseBase,
  BoardStateEnvelope,
  UserBoardItem,
} from '../../../../shared/api/generated-query/model'
import { resolveApiData, resolveApiOk } from '../../../../shared/api/apiResponse'
import { i18n } from '../../../../shared/i18n'

export type { ApiResponseBase, BoardStateEnvelope, UserBoardItem }

export type SaveUserBoardPayload = {
  name: string
  state: BoardStateEnvelope
  base_revision: number
  created_at: number
  updated_at: number
}

export async function fetchUserBoards(): Promise<UserBoardItem[]> {
  const data = await resolveApiData(listUserBoards(), i18n.global.t('workspace.sync.listFailed'))
  return Array.isArray(data.list) ? data.list : []
}

export async function saveUserBoard(boardId: string, payload: SaveUserBoardPayload) {
  return resolveApiData(
    upsertUserBoard(boardId, payload),
    i18n.global.t('workspace.sync.saveFailed'),
  )
}

export async function removeUserBoard(boardId: string): Promise<void> {
  await resolveApiOk(deleteUserBoard(boardId), i18n.global.t('workspace.sync.deleteFailed'))
}

export async function joinUserBoardByShareToken(token: string) {
  return resolveApiData(
    joinBoardByShareToken({ token }),
    i18n.global.t('workspace.joinSharedBoard.joinFailed'),
  )
}
