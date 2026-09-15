import { describe, expect, it } from 'vitest'
import { bakeWebGLRetouchUniforms, createWebGLRetouchRenderer } from './webglEngine'
import type { ImageRetouchPlan } from './engine'

/**
 * Regression guard for the WebGL HSL hue-phase bug: the fragment shader once
 * reconstructed HSL with rotated channel phases, turning pink into purple and
 * green into yellow on every preview AND every applied file. Tolerances stay
 * generous for SwiftShader/mediump variance across CI GPUs.
 */
function makeQuadrantSource() {
  const canvas = document.createElement('canvas')
  canvas.width = 64
  canvas.height = 48
  const context = canvas.getContext('2d')
  expect(context).not.toBeNull()
  context!.fillStyle = 'rgb(210,41,138)'
  context!.fillRect(32, 0, 32, 24)
  context!.fillStyle = 'rgb(43,211,115)'
  context!.fillRect(0, 24, 32, 24)
  context!.fillStyle = 'rgb(43,41,43)'
  context!.fillRect(0, 0, 32, 24)
  context!.fillStyle = 'rgb(210,211,211)'
  context!.fillRect(32, 24, 32, 24)
  return canvas
}

function renderPlan(plan: ImageRetouchPlan) {
  const output = document.createElement('canvas')
  const renderer = createWebGLRetouchRenderer(output)
  expect(renderer).not.toBeNull()
  const source = makeQuadrantSource()
  renderer!.updateSourceImage(source, source.width, source.height)
  renderer!.updateUniforms(bakeWebGLRetouchUniforms(plan))
  renderer!.render()
  const pixels = renderer!.readPixels()
  renderer!.dispose()
  expect(pixels).not.toBeNull()
  const data = pixels!.data
  const at = (x: number, y: number) => {
    const index = (y * 64 + x) * 4
    return [data[index] ?? 0, data[index + 1] ?? 0, data[index + 2] ?? 0]
  }
  return { pink: at(48, 6), green: at(8, 36) }
}

describe('webglEngine HSL hue fidelity', () => {
  it('keeps pink pink and green green under a saturation boost', () => {
    const plan: ImageRetouchPlan = [
      { kind: 'hsl', params: { hueShift: 0, saturationScale: 1.5, lightnessOffset: 0 } },
    ]
    const output = renderPlan(plan)

    // Pink (210,41,138) must stay red-dominant, not rotate toward purple/blue.
    expect(output.pink[0]).toBeGreaterThan(190)
    expect(output.pink[1]).toBeLessThan(90)
    expect(output.pink[2]).toBeGreaterThan(90)
    expect(output.pink[2]).toBeLessThan(200)
    // Green (43,211,115) must stay green-dominant, not rotate toward yellow.
    expect(output.green[1]).toBeGreaterThan(190)
    expect(output.green[0]).toBeLessThan(90)
    expect(output.green[2]).toBeGreaterThan(60)
    expect(output.green[2]).toBeLessThan(180)
  })

  it('preserves saturated hues with an identity plan', () => {
    const output = renderPlan([])

    expect(output.pink[0]).toBeGreaterThan(170)
    expect(output.pink[1]).toBeLessThan(80)
    expect(output.green[1]).toBeGreaterThan(170)
    expect(output.green[0]).toBeLessThan(80)
  })
})
