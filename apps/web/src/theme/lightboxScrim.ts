// Shared scrim-button chrome for fullscreen lightbox overlays (search + library).
// Both lightboxes float frosted-glass actions over a dark scrim; the base trio
// (translucent fill, soft ink, hairline border) plus the hover inversion must
// stay identical across features. Layout (pill link vs round icon button) stays
// with the caller so this module never learns feature markup.
export const lightboxScrimButtonBaseClass =
  'rounded-pill bg-white/10 text-white/90 backdrop-blur-md border border-white/15 transition-[background-color,color,border-color] duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] hover:bg-white hover:text-black hover:border-transparent'

export const lightboxScrimLinkClass = `${lightboxScrimButtonBaseClass} flex h-7 items-center justify-center px-3 text-xs font-medium no-underline cursor-pointer`

export const lightboxScrimIconButtonClass = `${lightboxScrimButtonBaseClass} grid size-7 place-items-center focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-white/80 cursor-pointer`
