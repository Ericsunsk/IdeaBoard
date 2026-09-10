import { useAppToast } from '../../../../shared/composables/useAppToast'
import { i18n } from '../../../../shared/i18n'
import type {
  BoardCreateResult,
  BoardDeleteResult,
  BoardDuplicateResult,
} from '../../../board/application/catalog/commands'
import type { BoardRepositoryRenameResult } from '../../../board/application/catalog/repository'

type BoardToast = Pick<ReturnType<typeof useAppToast>, 'success' | 'warning' | 'error'>

type FailedCreateResult = Extract<BoardCreateResult, { ok: false }>
type FailedRenameResult = Extract<BoardRepositoryRenameResult, { ok: false }>
type RenameValidationFailure = Extract<
  FailedRenameResult,
  { reason: 'local-name-conflict' | 'invalid-name' }
>
type RenameSyncFailure = Extract<
  FailedRenameResult,
  { reason: 'remote-name-conflict' | 'sync-failed' }
>
type FailedDuplicateResult = Extract<BoardDuplicateResult, { ok: false }>
type FailedDeleteResult = Extract<BoardDeleteResult, { ok: false }>

export type BoardFeedback = {
  createFailed: (result: FailedCreateResult) => void
  renameValidationFailed: (result: RenameValidationFailure) => void
  renameSyncFailed: (result: RenameSyncFailure) => void
  renamed: () => void
  copyFailed: (result: FailedDuplicateResult) => void
  copied: () => void
  deleteFailed: (result: FailedDeleteResult) => void
  deleted: () => void
  catalogSyncFailed: () => void
  catalogReloaded: () => void
}

export function createBoardFeedback(appToast: BoardToast): BoardFeedback {
  const t = i18n.global.t
  return {
    createFailed(result) {
      appToast.error(
        result.reason === 'name-conflict'
          ? t('workspace.feedback.createNameConflict')
          : t('workspace.feedback.createSyncFailed'),
      )
    },
    renameValidationFailed(result) {
      appToast.warning(
        result.reason === 'local-name-conflict'
          ? t('workspace.feedback.nameAlreadyExists')
          : t('workspace.feedback.nameEmpty'),
      )
    },
    renameSyncFailed(result) {
      if (result.reason === 'remote-name-conflict' && result.hydrate?.ok) {
        appToast.error(t('workspace.feedback.renameConflictReloaded'))
        return
      }
      if (result.reason === 'sync-failed' && result.rollback.ok) {
        appToast.error(t('workspace.feedback.renameSyncFailedRolledBack'))
        return
      }
      appToast.error(
        result.reason === 'remote-name-conflict'
          ? t('workspace.feedback.renameConflictRecoveryIncomplete')
          : t('workspace.feedback.renameSyncFailedNoRollback'),
      )
    },
    renamed() {
      appToast.success(t('workspace.feedback.renamed'))
    },
    copyFailed(result) {
      if (result.reason === 'source-missing') {
        appToast.error(t('workspace.feedback.copyFailed'))
        return
      }
      appToast.error(
        result.reason === 'name-conflict'
          ? t('workspace.feedback.duplicateNameConflict')
          : t('workspace.feedback.duplicateSyncFailed'),
      )
    },
    copied() {
      appToast.success(t('workspace.feedback.duplicated'))
    },
    deleteFailed(result) {
      appToast.error(
        result.reason === 'sync-failed'
          ? t('workspace.feedback.deleteSyncFailed')
          : t('workspace.feedback.deleteFailed'),
      )
    },
    deleted() {
      appToast.success(t('workspace.feedback.deleted'))
    },
    catalogSyncFailed() {
      appToast.error(t('workspace.feedback.syncFailedRetry'))
    },
    catalogReloaded() {
      appToast.success(t('workspace.feedback.reloaded'))
    },
  }
}

export function useBoardFeedback() {
  return createBoardFeedback(useAppToast())
}
