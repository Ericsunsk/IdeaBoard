import { useAppToast } from '../../../../core/composables/useAppToast'
import { i18n } from '../../../../core/i18n'
import type {
  BoardCreateResult,
  BoardDeleteResult,
  BoardDuplicateResult,
} from '../../session/catalog/commands'
import type { BoardRepositoryRenameResult } from '../../session/catalog/repository'

type BoardToast = Pick<ReturnType<typeof useAppToast>, 'success' | 'warning' | 'error'>

type FailedCreateResult = Extract<BoardCreateResult, { ok: false }>
type FailedRenameResult = Extract<BoardRepositoryRenameResult, { ok: false }>
type RenameValidationFailure = Extract<
  FailedRenameResult,
  { reason: 'local-name-conflict' | 'invalid-name' }
>
type RenameSyncFailure = Extract<
  FailedRenameResult,
  { reason: 'revision-conflict' | 'idempotency-conflict' | 'sync-failed' }
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
        result.reason === 'revision-conflict' || result.reason === 'idempotency-conflict'
          ? t('workspace.feedback.createSyncConflict')
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
      const isConflict =
        result.reason === 'revision-conflict' || result.reason === 'idempotency-conflict'
      if (isConflict && result.hydrate?.ok) {
        appToast.error(t('workspace.feedback.renameSyncConflictReloaded'))
        return
      }
      if (result.reason === 'sync-failed' && result.rollback.ok) {
        appToast.error(t('workspace.feedback.renameSyncFailedRolledBack'))
        return
      }
      appToast.error(
        isConflict
          ? t('workspace.feedback.renameSyncConflictRecoveryIncomplete')
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
        result.reason === 'revision-conflict' || result.reason === 'idempotency-conflict'
          ? t('workspace.feedback.duplicateSyncConflict')
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
