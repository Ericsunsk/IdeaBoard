// Share dialog skin: token-first classes over the shared dialog shell.
// Interactive controls reuse Button/Select primitives (touch-target floor
// included); these classes lay out rows, lists, and copy using canonical
// design tokens without artificial dividers.

export const shareDialogContentClass =
  'w-[440px] max-w-[calc(100vw-28px)] bg-popover shadow-[var(--shadow-board-dialog)]'

export const shareDialogClasses = {
  shell: 'flex min-w-0 flex-col gap-5 p-6',
  header: 'flex flex-col gap-1 pr-8',
  title: 'm-0 text-lg font-semibold leading-snug tracking-tight text-foreground truncate',
  statusLine: 'm-0 text-sm leading-normal text-muted-foreground',
  section: 'flex flex-col gap-2',
  sectionTitle: 'm-0 text-sm font-semibold text-foreground',
  loading: 'flex min-h-24 items-center justify-center',
  spinner: 'size-7 text-primary',
  errorBox:
    'flex flex-col items-start gap-2 rounded-control border border-destructive/30 bg-destructive/10 p-3 text-sm text-foreground',
  avatarRow: 'flex flex-wrap items-center gap-3 pt-1',
  avatarButton:
    'group relative flex cursor-pointer items-center justify-center rounded-pill p-0.5 outline-none transition-transform duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] focus-visible:ring-2 focus-visible:ring-ring active:scale-95 disabled:pointer-events-none',
  avatarCreatorBadge:
    'absolute -bottom-1 -right-1 flex size-4 items-center justify-center rounded-pill bg-primary text-xs font-bold text-primary-foreground shadow-xs ring-1 ring-background',
  avatarGrantedBadge:
    'absolute -bottom-1 -right-1 flex size-4 items-center justify-center rounded-pill bg-primary text-primary-foreground shadow-xs ring-1 ring-background',
  emptyNote: 'm-0 text-xs text-muted-foreground',
  accessButtonGroup: 'flex items-center gap-2',
} as const
