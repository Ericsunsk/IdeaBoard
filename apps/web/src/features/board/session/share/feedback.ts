import { useAppToast } from '../../../../core/composables/useAppToast'
import { i18n } from '../../../../core/i18n'
import type { ShareFailureReason } from './commands'

type ShareToast = Pick<ReturnType<typeof useAppToast>, 'success' | 'error' | 'warning'>

export type ShareFeedback = {
  accessUpdated: () => void
  actionFailed: (reason: ShareFailureReason) => void
}

export function createShareFeedback(appToast: ShareToast): ShareFeedback {
  const t = i18n.global.t
  return {
    accessUpdated() {
      appToast.success(t('board.shareDialog.accessUpdated'))
    },
    actionFailed(reason) {
      const key = failureCopyKey(reason)
      if (reason === 'access-conflict' || reason === 'precondition-required') {
        appToast.warning(t(key))
        return
      }
      appToast.error(t(key))
    },
  }
}

/** Failure reason to `board.shareDialog.*` copy key (single mapping). */
export function failureCopyKey(reason: ShareFailureReason): string {
  switch (reason) {
    case 'board-not-saved':
      return 'board.shareDialog.boardNotSaved'
    case 'not-found':
      return 'board.shareDialog.boardNotFound'
    case 'forbidden':
      return 'board.shareDialog.noPermission'
    case 'access-conflict':
      return 'board.shareDialog.accessConflict'
    case 'precondition-required':
      return 'board.shareDialog.preconditionRequired'
    case 'archived':
      return 'board.shareDialog.boardArchived'
    case 'member-unknown':
      return 'board.shareDialog.memberUnknown'
    case 'invalid':
      return 'board.shareDialog.invalidRequest'
    case 'sync-failed':
      return 'board.shareDialog.actionFailed'
  }
}

export function useShareFeedback(): ShareFeedback {
  return createShareFeedback(useAppToast())
}
