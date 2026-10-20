// Shared button chrome over the fixed dark media scrim (Search + Library).
// Media foreground stays light in both app themes; action fills and motion
// reuse the shared overlay roles. Layout remains with the caller.
export const lightboxScrimButtonBaseClass =
  'rounded-pill bg-overlay-btn-bg text-media-foreground/90 backdrop-blur-md border border-white/15 transition-[background-color,color,border-color] duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] hover:bg-overlay-btn-bg-hover hover:text-media-foreground hover:border-transparent'

export const lightboxScrimLinkClass = `${lightboxScrimButtonBaseClass} flex h-7 items-center justify-center px-3 text-xs font-medium no-underline cursor-pointer`

export const lightboxScrimIconButtonClass = `${lightboxScrimButtonBaseClass} grid size-7 place-items-center focus-visible:outline-none focus-visible:shadow-focus-ring cursor-pointer`
