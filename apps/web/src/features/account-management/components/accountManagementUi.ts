import type { DialogPassThroughOptions } from 'primevue/dialog'
import { ibDialogMaskPt } from '../../../shared/theme/dialogMask'
import { createDialogMotionPt } from '../../../shared/motion/dialogMotion'

export const accountManagementDialogStyle = {
  width: 'min(1120px, calc(100vw - 32px))',
  '--ib-dialog-motion-from-transform': 'translateY(8px) scale(0.99)',
} as const

export function createAccountManagementDialogPt(): DialogPassThroughOptions {
  return {
    ...ibDialogMaskPt,
    root: {
      class:
        '!max-h-[calc(100dvh-32px)] !overflow-hidden !rounded-[var(--radius-ib-overlay)] !border !border-ib-border-soft !bg-white !shadow-[var(--shadow-ib-overlay)] max-[640px]:!max-h-[calc(100dvh-20px)]',
    },
    content: {
      class: '!overflow-hidden !bg-white !p-0',
    },
    ...createDialogMotionPt(),
  }
}

export const accountMainContentClass =
  'relative grid h-[min(78dvh,720px)] min-h-0 grid-cols-[234px_minmax(0,1fr)] overflow-hidden bg-white max-[820px]:h-[calc(100dvh-64px)] max-[820px]:grid-cols-1 max-[820px]:grid-rows-[auto_minmax(0,1fr)]'

export const accountContentPaneClass =
  'min-h-0 overflow-x-hidden overflow-y-auto rounded-l-[var(--radius-ib-overlay)] bg-ib-surface-muted px-8 py-7 pr-14 [scrollbar-width:none] max-[820px]:rounded-l-none max-[820px]:rounded-t-[var(--radius-ib-overlay)] max-[820px]:px-4 max-[820px]:py-5 max-[820px]:pr-14'

export const accountCloseButtonClass =
  '!absolute !top-4 !right-4 !z-20 !flex !h-8 !w-8 !min-w-8 !items-center !justify-center !rounded-[var(--radius-ib-pill)] !border !border-transparent !bg-transparent !p-0 !text-ib-text-muted !shadow-none !transition-[background-color,border-color,color] !duration-[var(--motion-feedback-duration)] !ease-[var(--motion-feedback-easing)] hover:!border-ib-border-soft hover:!bg-ib-surface-hover hover:!text-ib-text focus-visible:!outline-none focus-visible:!shadow-[var(--shadow-ib-focus)]'

const accountNavTabBaseClass =
  '!flex !h-11 !w-full !justify-start !gap-2 !rounded-[var(--radius-ib-control)] !border-0 !px-2.5 !text-[14px] !font-semibold !shadow-none !transition-[background-color,color,box-shadow] !duration-[var(--motion-control-duration)] !ease-[var(--motion-control-easing)] focus-visible:!outline-none focus-visible:!shadow-[var(--shadow-ib-focus)]'

export function accountNavTabClass(active: boolean): string {
  return active
    ? `${accountNavTabBaseClass} !bg-ib-creative-accent-soft !text-ib-creative-accent-pressed`
    : `${accountNavTabBaseClass} !bg-transparent !text-ib-text-muted hover:!bg-ib-surface-hover hover:!text-ib-text`
}

export const accountLogoutButtonClass =
  '!flex !h-11 !w-full !justify-start !gap-2 !rounded-[var(--radius-ib-control)] !border-0 !bg-transparent !px-2.5 !text-[14px] !font-semibold !text-ib-danger-pressed !shadow-none !transition-[background-color,color,opacity] !duration-[var(--motion-feedback-duration)] !ease-[var(--motion-feedback-easing)] hover:!bg-ib-danger/5 hover:!text-ib-danger-pressed focus-visible:!outline-none focus-visible:!shadow-[var(--shadow-ib-focus)] disabled:!cursor-not-allowed disabled:!opacity-60'

export const accountCardPanelClass =
  'rounded-[var(--radius-ib-panel)] border border-ib-border-soft bg-ib-surface p-5 shadow-[var(--shadow-ib-control)] max-[640px]:p-4'

export const accountDataTableFrameClass =
  'rounded-[var(--radius-ib-panel)] border border-ib-border-soft bg-ib-surface shadow-[var(--shadow-ib-control)]'

export const accountPrimaryActionButtonClass =
  'inline-flex h-11 w-full items-center justify-center rounded-[var(--radius-ib-control)] border border-transparent bg-ib-creative-accent-hover px-5 text-sm font-semibold text-ib-text-inverse shadow-[var(--shadow-ib-control)] transition-[background-color,color,box-shadow] duration-[var(--motion-feedback-duration)] ease-[var(--motion-feedback-easing)] hover:bg-ib-creative-accent-pressed focus-visible:outline-none focus-visible:shadow-[var(--shadow-ib-focus)] disabled:cursor-not-allowed disabled:border-ib-border-soft disabled:bg-ib-surface-disabled disabled:text-ib-text-subtle disabled:shadow-none'

export const accountButtonPt = {
  label: { class: 'leading-none' },
  loadingIcon: { class: 'mr-2 size-4 animate-spin' },
} as const
