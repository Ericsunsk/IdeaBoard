import { joinBoardByShareToken } from '../../../shared/api/generated-query/boards/boards'
import { resolveApiData } from '../../../shared/api/apiResponse'
import { i18n } from '../../../shared/i18n'
import { useCanvasBoardCatalog } from './canvasBoardCatalog'

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
  const boardCatalog = useCanvasBoardCatalog()
  const normalizedToken = normalizeRequiredText(token, i18n.global.t('workspace.joinSharedBoard.missingToken'))
  const data = await resolveApiData(
    joinBoardByShareToken({ token: normalizedToken }),
    i18n.global.t('workspace.joinSharedBoard.joinFailed'),
  )
  const boardId = normalizeRequiredText(data.board?.board_id, i18n.global.t('workspace.joinSharedBoard.missingBoardId'))

  const hydrateResult = await boardCatalog.commands.hydrate()
  if (!hydrateResult.ok) {
    throw new Error(i18n.global.t('workspace.joinSharedBoard.hydrateFailed'), { cause: hydrateResult.error })
  }

  return { boardId }
}
