import { describe, expect, it } from 'vitest'
import {
  bakeWebGLRetouchUniforms,
  createIdentityWebGLUniforms,
  WEBGL_RETOUCH_FRAGMENT_SHADER,
  WEBGL_RETOUCH_VERTEX_SHADER,
} from './webglEngine'

describe('webglRetouchEngine uniforms', () => {
  it('ships vertex and fragment shaders with a main entry', () => {
    expect(WEBGL_RETOUCH_VERTEX_SHADER).toContain('void main')
    expect(WEBGL_RETOUCH_FRAGMENT_SHADER).toContain('void main')
    expect(WEBGL_RETOUCH_FRAGMENT_SHADER).toContain('u_hsl')
  })

  it('bakes an empty plan to identity uniforms', () => {
    expect(bakeWebGLRetouchUniforms([])).toEqual(createIdentityWebGLUniforms())
  })

  it('combines hsl stacks and composes channel luts without CPU pixels', () => {
    const red = new Uint8ClampedArray(256).map((_, index) => Math.min(255, index + 10))
    const green = new Uint8ClampedArray(256).map((_, index) => index)
    const blue = new Uint8ClampedArray(256).map((_, index) => index)
    const uniforms = bakeWebGLRetouchUniforms([
      { kind: 'hsl', params: { hueShift: 0.1, saturationScale: 1.5, lightnessOffset: 0.05 } },
      { kind: 'hsl', params: { hueShift: 0.2, saturationScale: 2, lightnessOffset: -0.02 } },
      { kind: 'channel-lut', luts: { red, green, blue } },
      { kind: 'threshold', threshold: 128, invert: false },
      {
        kind: 'sharpen',
        params: { amount: 50, radius: 1, threshold: 4, protectColor: false },
      },
    ])
    expect(uniforms.hueShift).toBeCloseTo(0.3)
    expect(uniforms.saturationScale).toBeCloseTo(3)
    expect(uniforms.lightnessOffset).toBeCloseTo(0.03)
    expect(uniforms.lut?.red[0]).toBe(10)
    expect(uniforms.thresholdEnabled).toBe(true)
    expect(uniforms.sharpenEnabled).toBe(true)
    expect(uniforms.sharpenAmount).toBe(50)
  })

  it('keeps the last threshold and sharpen layers', () => {
    const uniforms = bakeWebGLRetouchUniforms([
      { kind: 'threshold', threshold: 100, invert: false },
      { kind: 'threshold', threshold: 180, invert: true },
    ])
    expect(uniforms.threshold).toBe(180)
    expect(uniforms.thresholdInvert).toBe(true)
  })
})
