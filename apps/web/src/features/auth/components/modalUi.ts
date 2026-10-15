import {
  ibFormInputClass,
  ibFormInputErrorClass,
  ibFormInputIconClass,
} from '../../../theme/formControls'
import './auth-modal-motion.css'

// Dialog 以 :class/cn() 组合造型：mask 沿用 Dialog 内置 ibDialogMaskClass，
// 动画走 reka data-state + Tailwind animate；m-4 在 reka 居中定位下会偏移，不再保留。
export const authDialogContentClass =
  'w-[min(390px,calc(100vw-32px))] overflow-visible border-0 bg-transparent p-0 shadow-none'

const authModalShellBaseClass =
  'relative overflow-hidden rounded-overlay border border-[var(--color-auth-shell-border)] px-5 text-[var(--color-auth-text)] shadow-auth-shell sm:px-7'

export const authModalShellClass = `${authModalShellBaseClass} bg-popover py-6 sm:py-7`

export const authCompactModalShellClass = `${authModalShellBaseClass} bg-[var(--color-auth-shell)] backdrop-blur-xl py-5 sm:py-6`

export const authModalBackdropClass =
  'pointer-events-none absolute inset-0 bg-gradient-to-tr from-[var(--color-auth-backdrop-from)] via-[var(--color-auth-backdrop-via)] to-[var(--color-auth-backdrop-to)]'

export const authModalHeaderClass = 'relative z-[1] mb-5 text-center'

export const authModalKickerClass =
  'mb-1 text-admin-caption font-normal uppercase tracking-[0.14em] text-[var(--color-auth-link)] font-hand'

export const authModalTitleClass =
  'm-0 text-admin-icon font-bold leading-tight tracking-tight text-[var(--color-auth-title)] font-display sm:text-admin-modal-title-mobile'

export const authFieldClass =
  'relative z-[1] flex min-w-0 flex-col gap-1 text-[12.5px] font-semibold text-[var(--color-auth-subtle)]'

export const authInputIconClass = ibFormInputIconClass

export const authInputClass = `${ibFormInputClass} !text-admin-content`

export const authInputErrorClass = ibFormInputErrorClass

export const authPrimaryButtonClass =
  'control-state auth-primary-action [--control-hover-transform:scale(1.008)] [--control-active-transform:scale(0.99)] [--control-disabled-opacity:0.7] mt-1 inline-flex h-[42px] w-full cursor-pointer items-center justify-center rounded-control border-0 bg-gradient-to-r from-[var(--color-auth-primary-from)] to-[var(--color-auth-primary-to)] px-6 text-admin-content font-semibold text-[var(--color-auth-primary-text)] shadow-auth-primary transition-[transform,box-shadow,opacity] duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] hover:shadow-auth-primary-hover hover:opacity-90'

export const authSocialIconButtonClass =
  'control-state [--control-active-transform:scale(0.99)] [--control-disabled-opacity:0.7] inline-flex h-10 w-full items-center justify-center rounded-control border border-border bg-card p-0 shadow-none outline-none transition-[border-color,background-color,box-shadow,transform,opacity] duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] hover:border-input focus-visible:border-ring focus-visible:bg-card focus-visible:ring-1 focus-visible:ring-ring disabled:bg-muted'

export const authFooterClass =
  'relative z-[1] text-center text-admin-meta text-[var(--color-auth-muted)]'

export const authSwitchButtonClass =
  'control-state [--control-disabled-opacity:0.7] -my-1 ml-1.5 inline-flex min-h-8 cursor-pointer items-center border-0 bg-transparent px-1 text-[inherit] font-semibold text-[var(--color-auth-link)] transition-colors duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] hover:text-[var(--color-auth-link-hover)] hover:underline underline-offset-4'

export const authCloseButtonClass =
  'control-state auth-close-action [--control-active-transform:scale(0.95)] [--control-disabled-opacity:0.5] absolute top-3 right-3.5 z-[2] inline-flex size-8 items-center justify-center border-0 bg-transparent p-0 text-[var(--color-auth-close)] shadow-none transition-[color,transform,opacity] duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] hover:text-[var(--color-auth-close-hover)]'
