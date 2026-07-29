import type { DialogPassThroughOptions } from 'primevue/dialog'
import {
  createIbPasswordPt,
  ibFormErrorMessagePt,
  ibFormInputClass,
  ibFormInputErrorClass,
  ibFormInputIconClass,
} from '../../../shared/theme/formControls'
import { ibDialogMaskPt } from '../../../shared/theme/dialogMask'
import { createDialogMotionPt } from '../../../shared/motion/dialogMotion'
import './authModalMotion.css'

export function createAuthDialogPt() {
  return {
    ...ibDialogMaskPt,
    root: {
      class: 'm-5 w-[min(460px,calc(100vw-32px))] overflow-visible bg-transparent p-0',
      style: {
        '--ib-dialog-motion-from-transform': 'translateY(12px) scale(0.985)',
      },
    },
    content: {
      class: 'overflow-visible bg-transparent p-0',
    },
    ...createDialogMotionPt(),
  } satisfies DialogPassThroughOptions
}

export const authErrorMessagePt = ibFormErrorMessagePt

export const authButtonPt = {
  label: { class: 'leading-none' },
  loadingIcon: { class: 'mr-2 size-4 animate-spin' },
} as const

export const authPasswordPt = createIbPasswordPt('pl-11 pr-12')

const authModalShellBaseClass =
  'relative overflow-hidden rounded-[24px] border border-[var(--color-ib-auth-shell-border)] bg-[var(--color-ib-auth-shell)] backdrop-blur-xl px-11 text-[var(--color-ib-auth-text)] shadow-[var(--shadow-ib-auth-shell)] max-[520px]:px-7 max-[520px]:py-8'

export const authModalShellClass = `${authModalShellBaseClass} py-11`

export const authCompactModalShellClass = `${authModalShellBaseClass} py-9`

export const authModalBackdropClass =
  'pointer-events-none absolute inset-0 bg-gradient-to-tr from-[var(--color-ib-auth-backdrop-from)] via-[var(--color-ib-auth-backdrop-via)] to-[var(--color-ib-auth-backdrop-to)]'

export const authModalHeaderClass = 'relative z-[1] mb-8 text-center'

export const authModalKickerClass =
  "mb-2 text-[12px] font-normal uppercase tracking-[0.16em] text-[var(--color-ib-auth-link)] [font-family:'Caveat',cursive,sans-serif]"

export const authModalTitleClass =
  "m-0 text-[32px] font-bold leading-tight tracking-tight text-[var(--color-ib-auth-title)] [font-family:'Satoshi',-apple-system,'SF_Pro_Display',sans-serif] max-[520px]:text-[28px]"

export const authModalSubtitleClass =
  'mt-2 mb-0 text-[14px] leading-relaxed text-[var(--color-ib-auth-muted)]'

export const authFieldClass =
  'relative z-[1] flex min-w-0 flex-col gap-1.5 text-[11px] font-bold uppercase tracking-[0.1em] text-[var(--color-ib-auth-subtle)]'

export const authInputIconClass = ibFormInputIconClass

export const authInputClass = ibFormInputClass

export const authInputErrorClass = ibFormInputErrorClass

export const authPrimaryButtonClass =
  'ib-control-state ib-auth-primary-action [--ib-control-hover-transform:scale(1.01)] [--ib-control-active-transform:scale(0.99)] [--ib-control-disabled-opacity:0.7] mt-2 inline-flex h-[50px] w-full items-center justify-center rounded-[12px] border-0 bg-gradient-to-r from-[var(--color-ib-auth-primary-from)] to-[var(--color-ib-auth-primary-to)] px-8 text-[15px] font-semibold text-[var(--color-ib-auth-primary-text)] shadow-[var(--shadow-ib-auth-primary)] transition-[transform,box-shadow,opacity] duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] hover:shadow-[var(--shadow-ib-auth-primary-hover)]'

export const authFooterClass =
  'relative z-[1] text-center text-[13px] text-[var(--color-ib-auth-muted)]'

export const authSwitchButtonClass =
  'ib-control-state [--ib-control-disabled-opacity:0.7] ml-1.5 cursor-pointer border-0 bg-transparent p-0 text-[inherit] font-semibold text-[var(--color-ib-auth-link)] transition-colors duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] hover:text-[var(--color-ib-auth-link-hover)] hover:underline underline-offset-4'

export const authCloseButtonClass =
  'ib-control-state ib-auth-close-action [--ib-control-active-transform:scale(0.95)] [--ib-control-disabled-opacity:0.5] absolute top-5 right-5 z-[2] inline-flex size-8 items-center justify-center border-0 bg-transparent p-0 text-[var(--color-ib-auth-close)] shadow-none transition-[color,transform,opacity] duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] hover:text-[var(--color-ib-auth-close-hover)]'
