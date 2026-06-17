import {
  deleteUserBoard,
  joinBoardByShareToken,
  listUserBoards,
  upsertUserBoard,
} from './generated-query/boards/boards'
import type { UserBoardUpsertRequest } from './generated-query/model'

export type { BoardStateEnvelope, UserBoardItem, UserBoardUpsertRequest } from './generated-query/model'
export type UpsertBoardPayload = UserBoardUpsertRequest
export type UserBoardPayload = UpsertBoardPayload & { board_id: string }

export const boardApi = {
  list: listUserBoards,
  upsert: upsertUserBoard,
  remove: deleteUserBoard,
  joinByShareToken: (token: string) => joinBoardByShareToken({ token }),
}

export { deleteUserBoard as deleteBoard, joinBoardByShareToken, listUserBoards, upsertUserBoard }

export default boardApi
