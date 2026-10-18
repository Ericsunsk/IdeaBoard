import { beforeEach, describe, expect, test, vi } from 'vitest'
import { setUserLocale } from '../../../../core/i18n'
import { mockAppToast } from '../../../../test/toast'
import { createShareFeedback, failureCopyKey } from './feedback'

vi.mock('../../../../core/composables/useAppToast')

describe('share feedback', () => {
  beforeEach(async () => {
    await setUserLocale('en')
    mockAppToast()
  })

  test('maps every failure reason to a shareDialog copy key', () => {
    expect(failureCopyKey('board-not-saved')).toBe('board.shareDialog.boardNotSaved')
    expect(failureCopyKey('not-found')).toBe('board.shareDialog.boardNotFound')
    expect(failureCopyKey('forbidden')).toBe('board.shareDialog.noPermission')
    expect(failureCopyKey('access-conflict')).toBe('board.shareDialog.accessConflict')
    expect(failureCopyKey('precondition-required')).toBe('board.shareDialog.preconditionRequired')
    expect(failureCopyKey('archived')).toBe('board.shareDialog.boardArchived')
    expect(failureCopyKey('member-unknown')).toBe('board.shareDialog.memberUnknown')
    expect(failureCopyKey('invalid')).toBe('board.shareDialog.invalidRequest')
    expect(failureCopyKey('sync-failed')).toBe('board.shareDialog.actionFailed')
  })

  test('shows sharing success without an Undo action', () => {
    const toast = mockAppToast()
    createShareFeedback(toast).accessUpdated()
    expect(toast.success).toHaveBeenCalledExactlyOnceWith('Board sharing updated.')
  })

  test('conflicts warn while permission loss errors', () => {
    const toast = mockAppToast()
    const feedback = createShareFeedback(toast)

    feedback.actionFailed('access-conflict')
    expect(toast.warning).toHaveBeenCalledTimes(1)
    expect(toast.error).not.toHaveBeenCalled()

    feedback.actionFailed('forbidden')
    expect(toast.error).toHaveBeenCalledTimes(1)
  })
})
