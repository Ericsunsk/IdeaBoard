export const authDialogPt = {
  mask: {
    class: 'bg-[#0f111a]/40 backdrop-blur-[8px] transition-all duration-300',
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
    class: 'm-0 border-0 bg-transparent p-0 text-[12px] font-semibold text-rose-500 tracking-wide mt-1.5',
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
      'absolute top-1/2 right-4 size-5 -translate-y-1/2 cursor-pointer text-slate-400 transition-colors duration-200 hover:text-slate-700',
  },
  unmaskIcon: {
    class:
      'absolute top-1/2 right-4 size-5 -translate-y-1/2 cursor-pointer text-slate-400 transition-colors duration-200 hover:text-slate-700',
  },
} as const

export const authModalMotion = {
  initial: { opacity: 0, y: 24, scale: 0.96 },
  enter: {
    opacity: 1,
    y: 0,
    scale: 1,
    transition: {
      type: 'spring',
      stiffness: 150,
      damping: 18,
    },
  },
} as const

const authModalShellBaseClass =
  'relative overflow-hidden rounded-[24px] border border-white/40 bg-white/75 backdrop-blur-xl px-11 text-slate-800 shadow-[0_24px_50px_-15px_rgba(146,120,230,0.18),inset_0_1px_1px_rgba(255,255,255,0.6)] max-[520px]:px-7 max-[520px]:py-8 transition-all duration-300'

export const authModalShellClass = `${authModalShellBaseClass} py-11`

export const authCompactModalShellClass = `${authModalShellBaseClass} py-9`

export const authModalBackdropClass =
  'pointer-events-none absolute inset-0 bg-gradient-to-tr from-white/30 via-white/70 to-white/90'

export const authModalHeaderClass = 'relative z-[1] mb-8 text-center'

export const authModalKickerClass =
  "mb-2 text-[12px] font-normal uppercase tracking-[0.16em] text-[#9278e6] [font-family:'Comic_Sans_MS','Chalkboard_SE','Bradley_Hand','Caveat',cursive,sans-serif]"

export const authModalTitleClass =
  "m-0 text-[32px] font-bold leading-tight tracking-tight text-slate-900 [font-family:'Satoshi',-apple-system,'SF_Pro_Display',sans-serif] max-[520px]:text-[28px]"

export const authModalSubtitleClass = 'mt-2 mb-0 text-[14px] leading-relaxed text-slate-500'

export const authFieldClass =
  'relative z-[1] flex min-w-0 flex-col gap-1.5 text-[11px] font-bold uppercase tracking-[0.1em] text-slate-400'

export const authInputIconClass =
  'pointer-events-none absolute top-1/2 left-3.5 z-10 flex -translate-y-1/2 items-center justify-center text-slate-400'

export const authInputClass =
  'h-[50px] w-full rounded-[12px] border border-slate-200 bg-white/60 px-4 text-[15px] text-slate-800 shadow-[inset_0_1px_2px_rgba(0,0,0,0.02)] outline-none transition-all duration-200 placeholder:text-slate-400 hover:border-slate-300 focus:border-[#9278e6] focus:bg-white focus:ring-4 focus:ring-[#9278e6]/10'

export const authInputErrorClass =
  'border-rose-300 bg-rose-50/50 placeholder:text-rose-400 focus:border-rose-400 focus:ring-4 focus:ring-rose-400/10'

export const authPrimaryButtonClass =
  'mt-2 inline-flex h-[50px] w-full items-center justify-center rounded-[12px] border-0 bg-gradient-to-r from-[#9278e6] to-[#7c6be3] px-8 text-[15px] font-semibold text-white shadow-[0_4px_14px_rgba(146,120,230,0.25)] transition-all duration-300 hover:scale-[1.01] hover:shadow-[0_6px_20px_rgba(146,120,230,0.35)] active:scale-[0.99] disabled:scale-100 disabled:cursor-not-allowed disabled:opacity-70'

export const authFooterClass = 'relative z-[1] text-center text-[13px] text-slate-500'

export const authSwitchButtonClass =
  'ml-1.5 cursor-pointer border-0 bg-transparent p-0 text-[inherit] font-semibold text-[#9278e6] hover:text-[#7c6be3] transition-colors duration-200 hover:underline underline-offset-4'

export const authCloseButtonClass =
  'absolute top-5 right-5 z-[2] inline-flex size-8 items-center justify-center border-0 bg-transparent p-0 text-slate-500 shadow-none transition-colors duration-200 hover:text-slate-900 active:scale-95 disabled:cursor-not-allowed disabled:opacity-50'
