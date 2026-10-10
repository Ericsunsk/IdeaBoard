import type { UserConfirmRequest } from '../../../core/composables/useUserConfirmDialog'
import { i18n } from '../../../core/i18n'

export function deleteFolderConfirmation(folderName: string): UserConfirmRequest {
  return {
    title: i18n.global.t('library.dialogs.deleteFolderTitle'),
    message: i18n.global.t('library.dialogs.deleteFolderMessage', { name: folderName }),
    confirmLabel: i18n.global.t('library.dialogs.delete'),
    intent: 'danger',
  }
}

export function removeAssetConfirmation(): UserConfirmRequest {
  return {
    title: i18n.global.t('library.dialogs.removeAssetTitle'),
    message: i18n.global.t('library.dialogs.removeAssetMessage'),
    confirmLabel: i18n.global.t('library.dialogs.remove'),
    intent: 'danger',
  }
}
