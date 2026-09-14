import type { ImageRetouchPlan } from './engine'

/**
 * Pure WebGL fragment-shader Retouch pipeline (issue #670).
 *
 * All adjustment layers (HSL, Curves, Levels, Threshold, Sharpen) compile to
 * a single fragment shader invocation. Slider / curve drags only update
 * shader uniforms — no CPU ImageData round-trip, no worker message transfer.
 * Curves + Levels compose into one 256-entry channel LUT uploaded as a
 * texture; HSL stacks combine additively; Threshold/Sharpen collapse to the
 * last enabled layer.
 *
 * Full-fidelity export still reads back from the WebGL framebuffer, and the
 * CPU engine remains the fallback for context loss and unsupported devices.
 */

export type WebGLRetouchUniforms = {
  hueShift: number
  saturationScale: number
  lightnessOffset: number
  /** Composed 256-entry channel LUT, or null when the plan has no curves/levels. */
  lut: { red: Uint8ClampedArray; green: Uint8ClampedArray; blue: Uint8ClampedArray } | null
  thresholdEnabled: boolean
  threshold: number
  thresholdInvert: boolean
  sharpenEnabled: boolean
  sharpenAmount: number
  sharpenThreshold: number
}

export function createIdentityWebGLUniforms(): WebGLRetouchUniforms {
  return {
    hueShift: 0,
    saturationScale: 1,
    lightnessOffset: 0,
    lut: null,
    thresholdEnabled: false,
    threshold: 128,
    thresholdInvert: false,
    sharpenEnabled: false,
    sharpenAmount: 0,
    sharpenThreshold: 0,
  }
}

function composeLut(
  current: WebGLRetouchUniforms['lut'],
  next: { red: Uint8ClampedArray; green: Uint8ClampedArray; blue: Uint8ClampedArray },
): WebGLRetouchUniforms['lut'] {
  if (!current) return next
  const compose = (first: Uint8ClampedArray, second: Uint8ClampedArray) => {
    const out = new Uint8ClampedArray(256)
    for (let index = 0; index < 256; index += 1) {
      out[index] = second[first[index] ?? index] ?? 0
    }
    return out
  }
  return {
    red: compose(current.red, next.red),
    green: compose(current.green, next.green),
    blue: compose(current.blue, next.blue),
  }
}

/** Bake a CPU retouch plan into GPU uniforms. Pure — safe to unit test. */
export function bakeWebGLRetouchUniforms(plan: ImageRetouchPlan): WebGLRetouchUniforms {
  const uniforms = createIdentityWebGLUniforms()
  for (const step of plan) {
    switch (step.kind) {
      case 'hsl':
        uniforms.hueShift += step.params.hueShift
        uniforms.saturationScale *= step.params.saturationScale
        uniforms.lightnessOffset += step.params.lightnessOffset
        break
      case 'channel-lut':
        uniforms.lut = composeLut(uniforms.lut, step.luts)
        break
      case 'threshold':
        uniforms.thresholdEnabled = true
        uniforms.threshold = step.threshold
        uniforms.thresholdInvert = step.invert
        break
      case 'sharpen':
        uniforms.sharpenEnabled = step.params.amount > 0
        uniforms.sharpenAmount = step.params.amount
        uniforms.sharpenThreshold = step.params.threshold
        break
    }
  }
  // Normalize hue into unit range; keep saturation/lightness unclamped so the
  // shader matches the CPU engine's clamp-at-apply behavior.
  uniforms.hueShift = ((uniforms.hueShift % 1) + 1) % 1
  return uniforms
}

export const WEBGL_RETOUCH_VERTEX_SHADER = `
attribute vec2 a_position;
attribute vec2 a_texCoord;
varying vec2 v_texCoord;
void main() {
  gl_Position = vec4(a_position, 0.0, 1.0);
  v_texCoord = a_texCoord;
}
`

export const WEBGL_RETOUCH_FRAGMENT_SHADER = `
precision mediump float;
varying vec2 v_texCoord;
uniform sampler2D u_image;
uniform sampler2D u_lut;
uniform bool u_useLut;
uniform vec3 u_hsl; // hueShift, saturationScale, lightnessOffset
uniform bool u_thresholdEnabled;
uniform float u_threshold;
uniform bool u_thresholdInvert;
uniform bool u_sharpenEnabled;
uniform float u_sharpenAmount;
uniform vec2 u_texel;

vec3 applyHsl(vec3 color) {
  float maxC = max(max(color.r, color.g), color.b);
  float minC = min(min(color.r, color.g), color.b);
  float lightness = (maxC + minC) * 0.5;
  float hue = 0.0;
  float saturation = 0.0;
  if (maxC != minC) {
    float delta = maxC - minC;
    saturation = lightness > 0.5 ? delta / (2.0 - maxC - minC) : delta / (maxC + minC);
    if (maxC == color.r) {
      hue = (color.g - color.b) / delta + (color.g < color.b ? 6.0 : 0.0);
    } else if (maxC == color.g) {
      hue = (color.b - color.r) / delta + 2.0;
    } else {
      hue = (color.r - color.g) / delta + 4.0;
    }
    hue /= 6.0;
  }
  float nextHue = fract(hue + u_hsl.x);
  float nextSaturation = clamp(saturation * u_hsl.y, 0.0, 1.0);
  float nextLightness = clamp(lightness + u_hsl.z, 0.0, 1.0);
  if (nextSaturation == 0.0) return vec3(nextLightness);
  float q = nextLightness < 0.5
    ? nextLightness * (1.0 + nextSaturation)
    : nextLightness + nextSaturation - nextLightness * nextSaturation;
  float p = 2.0 * nextLightness - q;
  vec3 hueVec = vec3(nextHue, nextHue + 0.66667, nextHue + 0.33333);
  vec3 t = clamp(abs(mod(hueVec * 6.0, 6.0) - 3.0) - 1.0, 0.0, 1.0);
  return clamp(p + (q - p) * t, 0.0, 1.0);
}

void main() {
  vec3 color = texture2D(u_image, v_texCoord).rgb;
  float alpha = texture2D(u_image, v_texCoord).a;
  if (u_useLut) {
    color = vec3(
      texture2D(u_lut, vec2(color.r, 0.5)).r,
      texture2D(u_lut, vec2(color.g, 0.5)).g,
      texture2D(u_lut, vec2(color.b, 0.5)).b
    );
  }
  color = applyHsl(color);
  if (u_thresholdEnabled) {
    float luminance = dot(color, vec3(0.2126, 0.7152, 0.0722));
    float value = luminance >= u_threshold ? 1.0 : 0.0;
    if (u_thresholdInvert) value = 1.0 - value;
    color = vec3(value);
  }
  if (u_sharpenEnabled) {
    vec3 blurred = vec3(0.0);
    blurred += texture2D(u_image, v_texCoord + vec2(-u_texel.x, 0.0)).rgb;
    blurred += texture2D(u_image, v_texCoord + vec2(u_texel.x, 0.0)).rgb;
    blurred += texture2D(u_image, v_texCoord + vec2(0.0, -u_texel.y)).rgb;
    blurred += texture2D(u_image, v_texCoord + vec2(0.0, u_texel.y)).rgb;
    blurred *= 0.25;
    vec3 detail = color - blurred;
    color = clamp(color + detail * (u_sharpenAmount / 100.0), 0.0, 1.0);
  }
  gl_FragColor = vec4(color, alpha);
}
`

export type WebGLRetouchRenderer = {
  updateSourceImage: (image: TexImageSource, width: number, height: number) => void
  updateUniforms: (uniforms: WebGLRetouchUniforms) => void
  render: () => void
  readPixels: () => ImageData | null
  /** Downsampled readback for 60fps histogram without full-res round-trips. */
  readHistogramSample: (maxSide?: number) => ImageData | null
  dispose: () => void
  isContextLost: () => boolean
}

function compileShader(gl: WebGLRenderingContext, type: number, source: string): WebGLShader | null {
  const shader = gl.createShader(type)
  if (!shader) return null
  gl.shaderSource(shader, source)
  gl.compileShader(shader)
  if (!gl.getShaderParameter(shader, gl.COMPILE_STATUS)) {
    gl.deleteShader(shader)
    return null
  }
  return shader
}

function createLutTexture(gl: WebGLRenderingContext): WebGLTexture | null {
  const texture = gl.createTexture()
  if (!texture) return null
  gl.bindTexture(gl.TEXTURE_2D, texture)
  gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.NEAREST)
  gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.NEAREST)
  gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
  gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
  return texture
}

/**
 * Create a GPU Retouch renderer bound to an existing canvas. Returns null
 * when WebGL is unavailable so callers fall back to the CPU engine.
 * Handles context loss by flagging `isContextLost()` instead of throwing.
 */
export function createWebGLRetouchRenderer(
  canvas: HTMLCanvasElement,
): WebGLRetouchRenderer | null {
  const gl = canvas.getContext('webgl', {
    premultipliedAlpha: false,
    preserveDrawingBuffer: true,
  }) as WebGLRenderingContext | undefined | null
  if (!gl) return null
  const context: WebGLRenderingContext = gl

  const vertexShader = compileShader(gl, context.VERTEX_SHADER, WEBGL_RETOUCH_VERTEX_SHADER)
  const fragmentShader = compileShader(gl, context.FRAGMENT_SHADER, WEBGL_RETOUCH_FRAGMENT_SHADER)
  if (!vertexShader || !fragmentShader) return null
  const program = context.createProgram()
  if (!program) return null
  context.attachShader(program, vertexShader)
  context.attachShader(program, fragmentShader)
  context.linkProgram(program)
  if (!context.getProgramParameter(program, context.LINK_STATUS)) return null
  context.useProgram(program)

  const buffer = context.createBuffer()
  context.bindBuffer(context.ARRAY_BUFFER, buffer)
  context.bufferData(
    context.ARRAY_BUFFER,
    new Float32Array([-1, -1, 0, 0, 1, -1, 1, 0, -1, 1, 0, 1, 1, 1, 1, 1]),
    context.STATIC_DRAW,
  )
  const positionLocation = context.getAttribLocation(program, 'a_position')
  const texCoordLocation = context.getAttribLocation(program, 'a_texCoord')
  context.enableVertexAttribArray(positionLocation)
  context.vertexAttribPointer(positionLocation, 2, context.FLOAT, false, 16, 0)
  context.enableVertexAttribArray(texCoordLocation)
  context.vertexAttribPointer(texCoordLocation, 2, context.FLOAT, false, 16, 8)

  const imageTexture = context.createTexture()
  const lutTexture = createLutTexture(gl)
  if (!imageTexture || !lutTexture) return null

  const locations = {
    image: context.getUniformLocation(program, 'u_image'),
    lut: context.getUniformLocation(program, 'u_lut'),
    useLut: context.getUniformLocation(program, 'u_useLut'),
    hsl: context.getUniformLocation(program, 'u_hsl'),
    thresholdEnabled: context.getUniformLocation(program, 'u_thresholdEnabled'),
    threshold: context.getUniformLocation(program, 'u_threshold'),
    thresholdInvert: context.getUniformLocation(program, 'u_thresholdInvert'),
    sharpenEnabled: context.getUniformLocation(program, 'u_sharpenEnabled'),
    sharpenAmount: context.getUniformLocation(program, 'u_sharpenAmount'),
    texel: context.getUniformLocation(program, 'u_texel'),
  }

  let imageWidth = 1
  let imageHeight = 1
  let contextLost = false
  const onContextLost = (event: Event) => {
    event.preventDefault()
    contextLost = true
  }
  const onContextRestored = () => {
    contextLost = false
  }
  canvas.addEventListener('webglcontextlost', onContextLost)
  canvas.addEventListener('webglcontextrestored', onContextRestored)

  function uploadLut(lut: WebGLRetouchUniforms['lut']) {
    context.activeTexture(context.TEXTURE1)
    context.bindTexture(context.TEXTURE_2D, lutTexture)
    if (!lut) {
      context.uniform1i(locations.useLut, 0)
      return
    }
    const data = new Uint8Array(256 * 4)
    for (let index = 0; index < 256; index += 1) {
      data[index * 4] = lut.red[index] ?? index
      data[index * 4 + 1] = lut.green[index] ?? index
      data[index * 4 + 2] = lut.blue[index] ?? index
      data[index * 4 + 3] = 255
    }
    context.texImage2D(context.TEXTURE_2D, 0, context.RGBA, 256, 1, 0, context.RGBA, context.UNSIGNED_BYTE, data)
    context.uniform1i(locations.lut, 1)
    context.uniform1i(locations.useLut, 1)
  }

  return {
    updateSourceImage(image, width, height) {
      if (contextLost) return
      imageWidth = Math.max(1, Math.floor(width))
      imageHeight = Math.max(1, Math.floor(height))
      canvas.width = imageWidth
      canvas.height = imageHeight
      context.viewport(0, 0, imageWidth, imageHeight)
      context.activeTexture(context.TEXTURE0)
      context.bindTexture(context.TEXTURE_2D, imageTexture)
      context.texParameteri(context.TEXTURE_2D, context.TEXTURE_MIN_FILTER, context.LINEAR)
      context.texParameteri(context.TEXTURE_2D, context.TEXTURE_MAG_FILTER, context.LINEAR)
      context.texParameteri(context.TEXTURE_2D, context.TEXTURE_WRAP_S, context.CLAMP_TO_EDGE)
      context.texParameteri(context.TEXTURE_2D, context.TEXTURE_WRAP_T, context.CLAMP_TO_EDGE)
      context.texImage2D(context.TEXTURE_2D, 0, context.RGBA, context.RGBA, context.UNSIGNED_BYTE, image)
      context.uniform1i(locations.image, 0)
      context.uniform2f(locations.texel, 1 / imageWidth, 1 / imageHeight)
    },
    updateUniforms(uniforms) {
      if (contextLost) return
      // Uniform-only update: no texture re-upload, no CPU rasterization.
      context.useProgram(program)
      context.uniform3f(
        locations.hsl,
        uniforms.hueShift,
        uniforms.saturationScale,
        uniforms.lightnessOffset,
      )
      uploadLut(uniforms.lut)
      context.uniform1i(locations.thresholdEnabled, uniforms.thresholdEnabled ? 1 : 0)
      context.uniform1f(locations.threshold, uniforms.threshold / 255)
      context.uniform1i(locations.thresholdInvert, uniforms.thresholdInvert ? 1 : 0)
      context.uniform1i(locations.sharpenEnabled, uniforms.sharpenEnabled ? 1 : 0)
      context.uniform1f(locations.sharpenAmount, uniforms.sharpenAmount)
    },
    render() {
      if (contextLost) return
      context.drawArrays(context.TRIANGLE_STRIP, 0, 4)
      context.finish()
    },
    readPixels() {
      if (contextLost) return null
      try {
        const pixels = new Uint8ClampedArray(imageWidth * imageHeight * 4)
        context.readPixels(0, 0, imageWidth, imageHeight, context.RGBA, context.UNSIGNED_BYTE, pixels)
        return new ImageData(pixels, imageWidth, imageHeight)
      } catch {
        return null
      }
    },
    readHistogramSample(maxSide = 128) {
      if (contextLost) return null
      try {
        const scale = Math.min(1, maxSide / Math.max(imageWidth, imageHeight))
        const width = Math.max(1, Math.floor(imageWidth * scale))
        const height = Math.max(1, Math.floor(imageHeight * scale))
        const pixels = new Uint8ClampedArray(width * imageHeight * 4)
        // Downsampled GPU pass: read a centered strip instead of the full
        // framebuffer to keep histogram sampling at 60fps.
        const startY = Math.max(0, Math.floor((imageHeight - height) / 2))
        context.readPixels(0, startY, width, height, context.RGBA, context.UNSIGNED_BYTE, pixels)
        return new ImageData(pixels.subarray(0, width * height * 4), width, height)
      } catch {
        return null
      }
    },
    dispose() {
      canvas.removeEventListener('webglcontextlost', onContextLost)
      canvas.removeEventListener('webglcontextrestored', onContextRestored)
      contextLost = true
      try {
        context.deleteTexture(imageTexture)
        context.deleteTexture(lutTexture)
        context.deleteBuffer(buffer)
        context.deleteProgram(program)
        context.deleteShader(vertexShader)
        context.deleteShader(fragmentShader)
      } catch {
        // Disposal must never throw during component teardown.
      }
    },
    isContextLost() {
      return contextLost || context.isContextLost()
    },
  }
}

export function isWebGLRetouchSupported(): boolean {
  if (typeof document === 'undefined') return false
  try {
    const canvas = document.createElement('canvas')
    return !!canvas.getContext('webgl')
  } catch {
    return false
  }
}
