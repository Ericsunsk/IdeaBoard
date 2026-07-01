export const authDialogPt = {
  mask: {
    class: 'bg-[var(--color-ib-auth-mask)] backdrop-blur-[8px] transition-all duration-300',
  },
  root: {
    class: 'm-5 w-[min(420px,calc(100vw-32px))] overflow-visible bg-transparent p-0',
  },
  content: {
    class: 'overflow-visible bg-transparent p-0',
  },
} as const

export const authErrorMessagePt = {
  root: {
    class:
      'm-0 border-0 bg-transparent p-0 text-[12px] font-semibold text-[var(--color-ib-auth-error)] tracking-wide mt-1.5',
  },
  contentWrapper: { class: 'contents' },
  content: { class: 'contents' },
  text: { class: 'contents' },
} as const

export const authButtonPt = {
  label: { class: 'leading-none' },
  loadingIcon: { class: 'mr-2 size-4 animate-spin' },
} as const

export const authPasswordPt = {
  root: { class: 'relative block w-full' },
  pcInputText: {
    root: {
      class: 'pl-11 pr-12',
    },
  },
  maskIcon: {
    class:
      'absolute top-1/2 right-4 size-5 -translate-y-1/2 cursor-pointer text-[var(--color-ib-auth-input-icon)] transition-colors duration-200 hover:text-[var(--color-ib-auth-text)]',
  },
  unmaskIcon: {
    class:
      'absolute top-1/2 right-4 size-5 -translate-y-1/2 cursor-pointer text-[var(--color-ib-auth-input-icon)] transition-colors duration-200 hover:text-[var(--color-ib-auth-text)]',
  },
} as const

const authModalShellBaseClass =
  'relative overflow-hidden rounded-[24px] border border-[var(--color-ib-auth-shell-border)] bg-[var(--color-ib-auth-shell)] backdrop-blur-xl px-11 text-[var(--color-ib-auth-text)] shadow-[var(--shadow-ib-auth-shell)] max-[520px]:px-7 max-[520px]:py-8 transition-all duration-300'

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

export const authInputIconClass =
  'pointer-events-none absolute top-1/2 left-3.5 z-10 flex -translate-y-1/2 items-center justify-center text-[var(--color-ib-auth-input-icon)]'

export const authInputClass =
  'h-[50px] w-full rounded-[12px] border border-[var(--color-ib-auth-input-border)] bg-[var(--color-ib-auth-input)] px-4 text-[15px] text-[var(--color-ib-auth-text)] shadow-[var(--shadow-ib-auth-input)] outline-none transition-all duration-200 placeholder:text-[var(--color-ib-auth-input-placeholder)] hover:border-[var(--color-ib-auth-input-border-hover)] focus:border-[var(--color-ib-auth-focus)] focus:bg-[var(--color-ib-auth-input-focus)] focus:ring-4 focus:ring-[var(--color-ib-auth-focus-ring)]'

export const authInputErrorClass =
  'border-[var(--color-ib-auth-error-border)] bg-[var(--color-ib-auth-error-bg)] placeholder:text-[var(--color-ib-auth-error-placeholder)] focus:border-[var(--color-ib-auth-error)] focus:ring-4 focus:ring-[var(--color-ib-auth-error-ring)]'

export const authPrimaryButtonClass =
  'mt-2 inline-flex h-[50px] w-full items-center justify-center rounded-[12px] border-0 bg-gradient-to-r from-[var(--color-ib-auth-primary-from)] to-[var(--color-ib-auth-primary-to)] px-8 text-[15px] font-semibold text-[var(--color-ib-auth-primary-text)] shadow-[var(--shadow-ib-auth-primary)] transition-all duration-300 hover:scale-[1.01] hover:shadow-[var(--shadow-ib-auth-primary-hover)] active:scale-[0.99] disabled:scale-100 disabled:cursor-not-allowed disabled:opacity-70'

export const authFooterClass =
  'relative z-[1] text-center text-[13px] text-[var(--color-ib-auth-muted)]'

export const authSwitchButtonClass =
  'ml-1.5 cursor-pointer border-0 bg-transparent p-0 text-[inherit] font-semibold text-[var(--color-ib-auth-link)] transition-colors duration-200 hover:text-[var(--color-ib-auth-link-hover)] hover:underline underline-offset-4'

export const authCloseButtonClass =
  'absolute top-5 right-5 z-[2] inline-flex size-8 items-center justify-center border-0 bg-transparent p-0 text-[var(--color-ib-auth-close)] shadow-none transition-colors duration-200 hover:text-[var(--color-ib-auth-close-hover)] active:scale-95 disabled:cursor-not-allowed disabled:opacity-50'
