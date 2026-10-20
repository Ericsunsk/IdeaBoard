/// <reference types="node" />

import { describe, expect, it } from 'vitest'
import {
  lightboxScrimButtonBaseClass,
  lightboxScrimIconButtonClass,
  lightboxScrimLinkClass,
} from './lightboxScrim'

describe('Lightbox scrim button contract', () => {
  it('keeps the shared frosted-scrim trio and hover deepening in one place', () => {
    for (const token of ['bg-overlay-btn-bg', 'text-media-foreground/90', 'border-white/15']) {
      expect(lightboxScrimButtonBaseClass).toContain(token)
    }
    expect(lightboxScrimButtonBaseClass).toContain('backdrop-blur-md')
    expect(lightboxScrimButtonBaseClass).toContain('hover:bg-overlay-btn-bg-hover')
    expect(lightboxScrimButtonBaseClass).toContain('hover:text-media-foreground')
    expect(lightboxScrimButtonBaseClass).toContain('hover:border-transparent')
    expect(lightboxScrimButtonBaseClass).not.toContain('bg-white/10')
    expect(lightboxScrimButtonBaseClass).not.toContain('hover:bg-white')
    expect(lightboxScrimLinkClass).toContain(lightboxScrimButtonBaseClass)
    expect(lightboxScrimIconButtonClass).toContain(lightboxScrimButtonBaseClass)
  })

  it('keeps the close button keyboard focus ring on the shared variant', () => {
    expect(lightboxScrimIconButtonClass).toContain('focus-visible:shadow-focus-ring')
    expect(lightboxScrimIconButtonClass).not.toContain('focus-visible:ring-white/80')
  })
})
