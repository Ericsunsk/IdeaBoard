import {
  BoardPayloadError,
  BoardWorkspaceUnavailableError,
  joinWorkspaceBoardByShareToken,
} from '../adapters/boardsResource'
import { i18n } from '../../../../core/i18n'
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
  // The target workspace resolves from the selection store (share links
  // carry `?workspace=`); without a selection the join fails closed with
  // guidance instead of guessing a workspace for the bearer.
  let board: Awaited<ReturnType<typeof joinWorkspaceBoardByShareToken>>
  try {
    board = await joinWorkspaceBoardByShareToken(null, normalizedToken)
  } catch (error) {
    throw toJoinError(error)
  }
  const boardId = normalizeRequiredText(
    board.id,
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

function toJoinError(error: unknown): Error {
  // Fail-closed guidance and safe backend messages pass through; unsafe
  // transport payloads collapse to the workflow fallback (never leak).
  if (error instanceof BoardWorkspaceUnavailableError) return error
  // A well-formed transport with an unusable board envelope means the
  // server returned no usable canvas identity.
  if (error instanceof BoardPayloadError) {
    return new Error(i18n.global.t('workspace.joinSharedBoard.missingBoardId'), {
      cause: error,
    })
  }
  if (
    error &&
    typeof error === 'object' &&
    'hasSafeDisplayMessage' in error &&
    (error as { hasSafeDisplayMessage?: unknown }).hasSafeDisplayMessage === true
  ) {
    return error as unknown as Error
  }
  return new Error(i18n.global.t('workspace.joinSharedBoard.joinFailed'), { cause: error })
}
