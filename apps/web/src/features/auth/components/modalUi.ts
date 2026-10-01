import {
  ibFormInputClass,
  ibFormInputErrorClass,
  ibFormInputIconClass,
} from '../../../shared/theme/formControls'
import './auth-modal-motion.css'

// Dialog 以 :class/cn() 组合造型：mask 沿用 Dialog 内置 ibDialogMaskClass，
// 动画走 reka data-state + Tailwind animate；m-4 在 reka 居中定位下会偏移，不再保留。
export const authDialogContentClass =
  'w-[min(390px,calc(100vw-32px))] overflow-visible border-0 bg-transparent p-0 shadow-none'

const authModalShellBaseClass =
  'relative overflow-hidden rounded-[20px] border border-[var(--color-ib-auth-shell-border)] bg-[var(--color-ib-auth-shell)] backdrop-blur-xl px-5 text-[var(--color-ib-auth-text)] shadow-ib-auth-shell sm:px-7'

export const authModalShellClass = `${authModalShellBaseClass} py-6 sm:py-7`

export const authCompactModalShellClass = `${authModalShellBaseClass} py-5 sm:py-6`

export const authModalBackdropClass =
  'pointer-events-none absolute inset-0 bg-gradient-to-tr from-[var(--color-ib-auth-backdrop-from)] via-[var(--color-ib-auth-backdrop-via)] to-[var(--color-ib-auth-backdrop-to)]'

export const authModalHeaderClass = 'relative z-[1] mb-5 text-center'

export const authModalKickerClass =
  'mb-1 text-ib-caption font-normal uppercase tracking-[0.14em] text-[var(--color-ib-auth-link)] font-ib-hand'

export const authModalTitleClass =
  'm-0 text-ib-icon font-bold leading-tight tracking-tight text-[var(--color-ib-auth-title)] font-ib-display sm:text-ib-modal-title-mobile'

export const authFieldClass =
  'relative z-[1] flex min-w-0 flex-col gap-1 text-[12.5px] font-semibold text-[var(--color-ib-auth-subtle)]'

export const authInputIconClass = ibFormInputIconClass

export const authInputClass = `${ibFormInputClass} !text-ib-content`

export const authInputErrorClass = ibFormInputErrorClass

export const authPrimaryButtonClass =
  'ib-control-state ib-auth-primary-action [--ib-control-hover-transform:scale(1.008)] [--ib-control-active-transform:scale(0.99)] [--ib-control-disabled-opacity:0.7] mt-1 inline-flex h-[42px] w-full items-center justify-center rounded-ib-control border-0 bg-gradient-to-r from-[var(--color-ib-auth-primary-from)] to-[var(--color-ib-auth-primary-to)] px-6 text-ib-content font-semibold text-[var(--color-ib-auth-primary-text)] shadow-ib-auth-primary transition-[transform,box-shadow,opacity] duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] hover:shadow-ib-auth-primary-hover'

export const authSocialIconButtonClass =
  'ib-control-state [--ib-control-active-transform:scale(0.99)] [--ib-control-disabled-opacity:0.7] inline-flex h-10 w-full items-center justify-center rounded-[10px] border border-ib-border bg-ib-surface-raised p-0 shadow-none outline-none transition-[border-color,background-color,box-shadow,transform,opacity] duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] hover:border-ib-border-hover focus-visible:border-ib-creative-accent focus-visible:bg-ib-surface focus-visible:shadow-ib-focus disabled:bg-ib-surface-disabled'

export const authFooterClass =
  'relative z-[1] text-center text-ib-meta text-[var(--color-ib-auth-muted)]'

export const authSwitchButtonClass =
  'ib-control-state [--ib-control-disabled-opacity:0.7] -my-1 ml-1.5 inline-flex min-h-8 cursor-pointer items-center border-0 bg-transparent px-1 text-[inherit] font-semibold text-[var(--color-ib-auth-link)] transition-colors duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] hover:text-[var(--color-ib-auth-link-hover)] hover:underline underline-offset-4'

export const authCloseButtonClass =
  'ib-control-state ib-auth-close-action [--ib-control-active-transform:scale(0.95)] [--ib-control-disabled-opacity:0.5] absolute top-3 right-3.5 z-[2] inline-flex size-8 items-center justify-center border-0 bg-transparent p-0 text-[var(--color-ib-auth-close)] shadow-none transition-[color,transform,opacity] duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] hover:text-[var(--color-ib-auth-close-hover)]'
