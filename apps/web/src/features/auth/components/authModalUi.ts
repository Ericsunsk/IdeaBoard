export const authDialogPt = {
  mask: {
    class: 'bg-[rgba(21,27,43,0.24)] backdrop-blur-[6px]',
  },
  root: {
    class:
      'm-5 w-[min(440px,calc(100vw-40px))] overflow-visible rounded-[22px] border border-surface-200 bg-surface-0/90 shadow-[0_24px_40px_rgba(34,45,76,0.16)] backdrop-blur-[14px]',
  },
  content: {
    class: 'overflow-visible bg-transparent p-0',
  },
} as const

export const authModalMotion = {
  initial: { opacity: 0, y: 20, scale: 0.96 },
  enter: {
    opacity: 1,
    y: 0,
    scale: 1,
    transition: { duration: 260, easing: 'ease-out' },
  },
} as const

const authModalShellBaseClass = 'relative px-12 max-[520px]:px-8 max-[520px]:py-8'

export const authModalShellClass = `${authModalShellBaseClass} py-12`

export const authCompactModalShellClass = `${authModalShellBaseClass} py-10`

export const authModalTitleClass =
  'm-0 bg-[linear-gradient(135deg,var(--p-surface-900)_0%,var(--p-surface-700)_100%)] bg-clip-text text-[32px] leading-tight font-bold text-transparent'

export const authModalSubtitleClass = 'mt-2 mb-0 text-base text-surface-500'

export const authFieldClass = 'flex min-w-0 flex-col gap-2 text-sm font-medium text-surface-900'

export const authInputClass =
  'h-[52px] w-full rounded-[14px] border-surface-300 bg-surface-0/80 px-3 text-base text-surface-900'

export const authPrimaryButtonClass =
  'mt-1 h-[52px] rounded-2xl text-[17px] font-semibold shadow-[0_8px_24px_rgba(17,24,39,0.16)] transition hover:-translate-y-0.5 hover:shadow-[0_12px_32px_rgba(17,24,39,0.22)] active:translate-y-0'

export const authFooterClass = 'text-center text-[15px] text-surface-500'

export const authSwitchButtonClass =
  'ml-2 cursor-pointer border-0 bg-transparent p-0 text-[inherit] font-semibold text-primary-800 hover:underline disabled:cursor-not-allowed disabled:opacity-50'

export const authCloseButtonClass =
  '!absolute !top-6 !right-6 !size-8 !p-0 !text-surface-900 !shadow-none hover:!bg-surface-900/10 disabled:!cursor-not-allowed disabled:!opacity-50'
