// Dialog 内容区 chrome：mask 沿用 Dialog 内置 ibDialogMaskClass，
// 动画走 reka data-state + Tailwind animate。
export const accountDialogContentClass =
  'w-[min(1120px,calc(100vw-32px))] max-w-[min(1120px,calc(100vw-32px))] grid-rows-[minmax(0,1fr)] shadow-ib-overlay max-h-[calc(100dvh-20px)]'

export const accountMainContentClass =
  'relative grid h-[calc(100dvh-64px)] min-h-0 grid-cols-1 grid-rows-[auto_minmax(0,1fr)] overflow-hidden bg-white md:h-[min(78dvh,720px)] md:grid-cols-[234px_minmax(0,1fr)] md:grid-rows-none'

export const accountContentPaneClass =
  'min-h-0 overflow-x-hidden overflow-y-auto rounded-l-none rounded-t-ib-overlay bg-ib-surface-muted px-4 py-5 pr-14 [scrollbar-width:none] md:rounded-l-ib-overlay md:rounded-t-none md:px-8 md:py-7'

export const accountCloseButtonClass =
  '!absolute !top-4 !right-4 !z-20 !flex !h-8 !w-8 !min-w-8 !items-center !justify-center !rounded-ib-pill !border !border-transparent !bg-transparent !p-0 !text-ib-text-muted !shadow-none !transition-[background-color,border-color,color] !duration-[var(--motion-feedback-duration)] !ease-[var(--motion-feedback-easing)] hover:!border-ib-border-soft hover:!bg-ib-surface-hover hover:!text-ib-text focus-visible:!outline-none focus-visible:!shadow-ib-focus'

const accountNavTabBaseClass =
  '!flex !h-11 !w-full !justify-start !gap-2 !rounded-ib-control !border-0 !px-2.5 !text-ib-content !font-semibold !shadow-none !transition-[background-color,color,box-shadow] !duration-[var(--motion-control-duration)] !ease-[var(--motion-control-easing)] focus-visible:!outline-none focus-visible:!shadow-ib-focus'

export function accountNavTabClass(active: boolean): string {
  return active
    ? `${accountNavTabBaseClass} !bg-ib-creative-accent-soft !text-ib-creative-accent-pressed`
    : `${accountNavTabBaseClass} !bg-transparent !text-ib-text-muted hover:!bg-ib-surface-hover hover:!text-ib-text`
}

export const accountLogoutButtonClass =
  '!flex !h-11 !w-full !justify-start !gap-2 !rounded-ib-control !border-0 !bg-transparent !px-2.5 !text-ib-content !font-semibold !text-ib-danger-pressed !shadow-none !transition-[background-color,color,opacity] !duration-[var(--motion-feedback-duration)] !ease-[var(--motion-feedback-easing)] hover:!bg-ib-danger/5 hover:!text-ib-danger-pressed focus-visible:!outline-none focus-visible:!shadow-ib-focus disabled:!cursor-not-allowed disabled:!opacity-60'

export const accountCardPanelClass =
  'rounded-ib-panel border border-ib-border-soft bg-ib-surface p-4 shadow-ib-control sm:p-5'

export const accountDataTableFrameClass =
  'rounded-ib-panel border border-ib-border-soft bg-ib-surface shadow-ib-control'

export const accountPrimaryActionButtonClass =
  'inline-flex h-11 w-full items-center justify-center rounded-ib-control border border-transparent bg-ib-creative-accent-hover px-5 text-sm font-semibold text-ib-text-inverse shadow-ib-control transition-[background-color,color,box-shadow] duration-[var(--motion-feedback-duration)] ease-[var(--motion-feedback-easing)] hover:bg-ib-creative-accent-pressed focus-visible:outline-none focus-visible:shadow-ib-focus disabled:cursor-not-allowed disabled:border-ib-border-soft disabled:bg-ib-surface-disabled disabled:text-ib-text-subtle disabled:shadow-none'
