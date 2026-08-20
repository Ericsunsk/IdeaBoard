import type { UserConfirmRequest } from '../../shared/composables/useUserConfirmDialog'

export function deleteFolderConfirmation(folderName: string): UserConfirmRequest {
  return {
    title: 'Delete folder',
    message: `Delete "${folderName}"? Saved references will move to Quick Saves.`,
    confirmLabel: 'Delete',
    intent: 'danger',
  }
}

export function removeAssetConfirmation(): UserConfirmRequest {
  return {
    title: 'Remove saved reference',
    message: 'Remove this saved reference from your Library?',
    confirmLabel: 'Remove',
    intent: 'danger',
  }
}
