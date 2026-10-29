/**
 * Core dialog size contract: the single width source for every modal dialog.
 * Admin/User/Share/Auth presets all resolve through this map so `data-size`
 * stays comparable across surfaces. Widths match the Admin shell contract
 * (slim 23rem → wide 61.25rem). The 420px profiles retain the legacy
 * 32px viewport guard and original rem caps: slim-inset for confirm/create,
 * narrow for folder dialogs.
 */
export type DialogShellSize =
  'slim' | 'slim-inset' | 'narrow' | 'compact' | 'form' | 'medium' | 'wide'

const insetDialogWidthClass = 'w-[min(420px,calc(100vw-32px))]'

export const dialogShellSizeClasses: Record<DialogShellSize, string> = {
  slim: 'w-[min(23rem,calc(100vw-1.5rem))] max-w-[23rem]',
  'slim-inset': `${insetDialogWidthClass} max-w-[23rem]`,
  narrow: `${insetDialogWidthClass} max-w-[30rem]`,
  compact: 'w-[min(30rem,calc(100vw-1.5rem))] max-w-[30rem]',
  form: 'w-[min(35rem,calc(100vw-1.5rem))] max-w-[35rem]',
  medium: 'w-[min(46rem,calc(100vw-1.5rem))] max-w-[46rem]',
  wide: 'w-[min(61.25rem,calc(100vw-1.5rem))] max-w-[61.25rem]',
}

type DialogShellPreset = {
  size: DialogShellSize
  hasClose: boolean
}

type DialogShellTrack = 'share' | 'auth' | 'account' | 'confirm'

/**
 * Per-surface dialog presets. The shell stays neutral; each track picks a
 * size and close affordance. Alert semantics stay split by design:
 * - User confirm (`UserConfirmDialog`) is a plain `dialog` (blocking confirm,
 *   typed-confirm support, focus restores through the shared lifecycle).
 * - Admin confirm (`AdminConfirmDialog`) is an `alertdialog` (destructive
 *   operator confirm, error toast keeps the dialog open).
 * Neither preset may cross that role boundary.
 */
export const dialogShellPresets: Record<DialogShellTrack, DialogShellPreset> = {
  share: { size: 'compact', hasClose: true },
  auth: { size: 'slim', hasClose: false },
  account: { size: 'slim', hasClose: true },
  confirm: { size: 'slim', hasClose: false },
}
