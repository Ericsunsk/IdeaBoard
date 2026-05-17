import {
  deleteUserBoard,
  joinBoardByShareToken,
  listUserBoards,
  upsertUserBoard,
} from './generated-query/boards/boards'
import type { UserBoardUpsertRequest, UserBoardUpsertRequestState } from './generated-query/model'

export type { UserBoardItem, UserBoardUpsertRequest } from './generated-query/model'
export type UpsertBoardPayload = Omit<UserBoardUpsertRequest, 'state'> & { state: unknown }
export type UserBoardPayload = UpsertBoardPayload & { board_id: string }

export const boardApi = {
  list: listUserBoards,
  upsert: (boardId: string, payload: UpsertBoardPayload) =>
    upsertUserBoard(boardId, {
      ...payload,
      state: (payload.state ?? null) as UserBoardUpsertRequestState,
    }),
  remove: deleteUserBoard,
  joinByShareToken: (token: string) => joinBoardByShareToken({ token }),
}

export {
  deleteUserBoard as deleteBoard,
  joinBoardByShareToken,
  listUserBoards,
  upsertUserBoard,
}

export default boardApi
