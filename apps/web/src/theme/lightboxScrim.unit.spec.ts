/// <reference types="node" />

import { describe, expect, it } from 'vitest'
import {
  lightboxScrimButtonBaseClass,
  lightboxScrimIconButtonClass,
  lightboxScrimLinkClass,
} from './lightboxScrim'

describe('Lightbox scrim button contract', () => {
  it('keeps the shared frosted-scrim trio and hover inversion in one place', () => {
    for (const token of ['bg-white/10', 'text-white/90', 'border-white/15']) {
      expect(lightboxScrimButtonBaseClass).toContain(token)
    }
    expect(lightboxScrimButtonBaseClass).toContain('backdrop-blur-md')
    expect(lightboxScrimButtonBaseClass).toContain('hover:bg-white')
    expect(lightboxScrimButtonBaseClass).toContain('hover:text-black')
    expect(lightboxScrimButtonBaseClass).toContain('hover:border-transparent')
    expect(lightboxScrimLinkClass).toContain(lightboxScrimButtonBaseClass)
    expect(lightboxScrimIconButtonClass).toContain(lightboxScrimButtonBaseClass)
  })

  it('keeps the close button keyboard focus ring on the shared variant', () => {
    expect(lightboxScrimIconButtonClass).toContain('focus-visible:ring-2')
    expect(lightboxScrimIconButtonClass).toContain('focus-visible:ring-white/80')
  })
})
