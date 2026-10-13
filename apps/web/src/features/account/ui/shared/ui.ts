// Dialog 内容区 chrome：mask 沿用 Dialog 内置 ibDialogMaskClass，
// 动画走 reka data-state + Tailwind animate。
export const accountDialogContentClass =
  'w-[min(1120px,calc(100vw-32px))] max-w-[min(1120px,calc(100vw-32px))] grid-rows-[minmax(0,1fr)] shadow-xl max-h-[calc(100dvh-20px)]'

export const accountMainContentClass =
  'relative grid h-[calc(100dvh-64px)] min-h-0 grid-cols-1 grid-rows-[auto_minmax(0,1fr)] overflow-hidden bg-card md:h-[min(78dvh,720px)] md:grid-cols-[234px_minmax(0,1fr)] md:grid-rows-none'

export const accountContentPaneClass =
  'min-h-0 overflow-x-hidden overflow-y-auto rounded-l-none rounded-t-overlay bg-muted/40 px-4 py-5 pr-14 [scrollbar-width:none] md:rounded-l-overlay md:rounded-t-none md:px-8 md:py-7'

export const accountCloseButtonClass =
  '!absolute !top-4 !right-4 !z-20 !flex !h-8 !w-8 !min-w-8 !items-center !justify-center !rounded-pill !border !border-transparent !bg-transparent !p-0 !text-muted-foreground !shadow-none !transition-[background-color,border-color,color] !duration-[var(--motion-feedback-duration)] !ease-[var(--motion-feedback-easing)] hover:!border-border hover:!bg-muted hover:!text-foreground focus-visible:!outline-none focus-visible:!shadow-focus-ring'

const accountNavTabBaseClass =
  '!flex !h-11 !w-full !justify-start !gap-2 !rounded-control !border-0 !px-2.5 !text-admin-content !font-semibold !shadow-none !transition-[background-color,color,box-shadow] !duration-[var(--motion-control-duration)] !ease-[var(--motion-control-easing)] focus-visible:!outline-none focus-visible:!shadow-focus-ring'

export function accountNavTabClass(active: boolean): string {
  return active
    ? `${accountNavTabBaseClass} !bg-primary/10 !text-primary`
    : `${accountNavTabBaseClass} !bg-transparent !text-muted-foreground hover:!bg-muted hover:!text-foreground`
}

export const accountLogoutButtonClass =
  '!flex !h-11 !w-full !justify-start !gap-2 !rounded-control !border-0 !bg-transparent !px-2.5 !text-admin-content !font-semibold !text-destructive !shadow-none !transition-[background-color,color,opacity] !duration-[var(--motion-feedback-duration)] !ease-[var(--motion-feedback-easing)] hover:!bg-destructive/10 hover:!text-destructive focus-visible:!outline-none focus-visible:!shadow-focus-ring disabled:!cursor-not-allowed disabled:!opacity-60'

export const accountCardPanelClass =
  'rounded-panel border border-border bg-card p-4 shadow-sm sm:p-5'

export const accountDataTableFrameClass = 'rounded-panel border border-border bg-card shadow-sm'

export const accountPrimaryActionButtonClass =
  'inline-flex h-11 w-full items-center justify-center rounded-control border border-transparent bg-primary px-5 text-sm font-semibold text-primary-foreground shadow-sm transition-[background-color,color,box-shadow] duration-[var(--motion-feedback-duration)] ease-[var(--motion-feedback-easing)] hover:bg-primary-hover focus-visible:outline-none focus-visible:shadow-focus-ring disabled:cursor-not-allowed disabled:border-border disabled:bg-muted disabled:text-muted-foreground disabled:shadow-none'
