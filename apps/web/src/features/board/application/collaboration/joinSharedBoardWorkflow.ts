import { joinUserBoardByShareToken } from '../../infrastructure/api/boardsResource'
import { i18n } from '../../../../shared/i18n'
import { useBoardRepository } from '../catalog/repository'

export interface JoinSharedBoardResult {
  boardId: string
}

function normalizeRequiredText(value: unknown, errorMessage: string): string {
  const normalized = String(value ?? '').trim()
  if (!normalized) throw new Error(errorMessage)
  return normalized
}

export async function joinSharedBoardByToken(token: string): Promise<JoinSharedBoardResult> {
  const boardRepository = useBoardRepository()
  const normalizedToken = normalizeRequiredText(
    token,
    i18n.global.t('workspace.joinSharedBoard.missingToken'),
  )
  const data = await joinUserBoardByShareToken(normalizedToken)
  const boardId = normalizeRequiredText(
    data.board?.board_id,
    i18n.global.t('workspace.joinSharedBoard.missingBoardId'),
  )

  const hydrateResult = await boardRepository.commands.hydrate()
  if (!hydrateResult.ok) {
    throw new Error(i18n.global.t('workspace.joinSharedBoard.hydrateFailed'), {
      cause: hydrateResult.error,
    })
  }

  return { boardId }
}
