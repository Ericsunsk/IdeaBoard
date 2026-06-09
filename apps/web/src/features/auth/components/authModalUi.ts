export const authDialogPt = {
  mask: {
    class: 'bg-[rgba(21,27,43,0.24)] backdrop-blur-[6px]',
  },
  root: {
    class:
      'm-5 w-[min(440px,calc(100vw-40px))] overflow-visible rounded-[22px] border border-[#d9dfec] bg-white/90 shadow-[0_24px_40px_rgba(34,45,76,0.16)] backdrop-blur-[14px]',
  },
  content: {
    class: 'overflow-visible bg-transparent p-0',
  },
} as const

export const authModalShellClass =
  'relative px-12 py-12 max-[520px]:px-8 max-[520px]:py-8'

export const authCompactModalShellClass =
  'relative px-12 py-10 max-[520px]:px-8 max-[520px]:py-8'

export const authModalTitleClass =
  'm-0 bg-[linear-gradient(135deg,#2b3142_0%,#4f5a76_100%)] bg-clip-text text-[32px] leading-tight font-bold text-transparent'

export const authModalSubtitleClass = 'mt-2 mb-0 text-base text-[#86868b]'

export const authFieldClass = 'flex min-w-0 flex-col gap-2 text-sm font-medium text-[#1d1d1f]'

export const authInputClass =
  'h-[52px] w-full rounded-[14px] border-[#d2d2d7] bg-white/80 px-3 text-base text-[#1d1d1f]'

export const authPrimaryButtonClass =
  'mt-1 h-[52px] rounded-2xl border-0 bg-[#7a6bd5] text-[17px] font-semibold text-white shadow-[0_8px_24px_rgba(122,107,213,0.28)] transition hover:-translate-y-0.5 hover:bg-[#6d5fd0] hover:shadow-[0_12px_32px_rgba(122,107,213,0.36)] active:translate-y-0'

export const authFooterClass = 'text-center text-[15px] text-[#86868b]'

export const authSwitchButtonClass =
  'ml-2 cursor-pointer border-0 bg-transparent p-0 text-[inherit] font-semibold text-[#7a6bd5] hover:underline disabled:cursor-not-allowed disabled:opacity-50'
