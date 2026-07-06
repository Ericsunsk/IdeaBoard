import { joinBoardByShareToken } from '../../../shared/api/generated-query/boards/boards'
import { hydrateCanvasBoardsFromServer } from './boardSync'

export interface JoinSharedCanvasBoardResult {
  boardId: string
}

function normalizeRequiredText(value: unknown, errorMessage: string): string {
  const normalized = String(value ?? '').trim()
  if (!normalized) throw new Error(errorMessage)
  return normalized
}

export async function joinSharedCanvasBoardByToken(
  token: string,
): Promise<JoinSharedCanvasBoardResult> {
  const normalizedToken = normalizeRequiredText(token, '缺少分享令牌')
  const res = await joinBoardByShareToken({ token: normalizedToken })
  const boardId = normalizeRequiredText(res.data?.board?.board_id, '服务端未返回可用画布 ID')

  const hydrateResult = await hydrateCanvasBoardsFromServer()
  if (!hydrateResult.ok) {
    throw new Error('画布同步失败，请稍后重试', { cause: hydrateResult.error })
  }

  return { boardId }
}
