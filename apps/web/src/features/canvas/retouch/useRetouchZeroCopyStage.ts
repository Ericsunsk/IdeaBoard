import {
  computed,
  onBeforeUnmount,
  ref,
  shallowRef,
  watch,
  type ComputedRef,
  type Ref,
} from 'vue'
import { compileImageRetouchPlan } from './utils/retouchEngine'
import {
  bakeWebGLRetouchUniforms,
  createWebGLRetouchRenderer,
  isWebGLRetouchSupported,
  type WebGLRetouchRenderer,
} from './utils/retouchWebglEngine'
import { flattenVisibleRetouchEffectLayers } from './utils/retouchLayerEffects'
import type { RetouchLayer } from './types'

export type ZeroCopyStageStatus = 'ready' | 'error' | 'idle'

export type ZeroCopyStageHandle = {
  /** Bound by the stage presentation canvas. The GPU draws here directly. */
  canvasRef: Ref<HTMLCanvasElement | null>
  setCanvasRef: (element: Element | ComponentPublicInstance | null) => void
  isGpuActive: ComputedRef<boolean>
  stageStatus: ComputedRef<ZeroCopyStageStatus>
  histogramSample: Ref<ImageData | null>
}

type ComponentPublicInstance = { $el?: unknown }

type UseZeroCopyRetouchStageOptions = {
  isAdjustMode: Ref<boolean>
  layers: Ref<readonly RetouchLayer[]>
  /** DOM texture source: the already-rendered node image on screen. */
  resolveDomImage: () => HTMLImageElement | null
}

const HISTOGRAM_SAMPLE_THROTTLE_MS = 250

/** A DOM image is texture-ready when decoded with a real size. */
export function isUsableTextureImage(image: HTMLImageElement | null | undefined): boolean {
  if (!image) return false
  return image.complete && image.naturalWidth > 0 && image.naturalHeight > 0
}

/**
 * Find the already-loaded node image inside a stage element. The Retouch
 * editor binds this element as its WebGL texture, so entering Retouch never
 * waits on the network.
 */
export function resolveDomTextureImage(stageElement: Element | null | undefined): HTMLImageElement | null {
  if (!stageElement || typeof stageElement.querySelector !== 'function') return null
  const images = Array.from(stageElement.querySelectorAll('img'))
  let best: HTMLImageElement | null = null
  let bestArea = 0
  for (const image of images) {
    if (!isUsableTextureImage(image)) continue
    const area = image.naturalWidth * image.naturalHeight
    if (area > bestArea) {
      bestArea = area
      best = image
    }
  }
  return best
}

/**
 * Zero-copy interactive Retouch stage (issues #672, #673).
 *
 * The WebGL canvas mounted by the stage presentation IS the render target:
 * slider/curve drags bake uniforms and schedule a single rAF draw. No
 * ImageData is read back during interaction. Texture binding relies solely on
 * the DOM image already on screen (0ms start). When unavailable or lost, the
 * stage transitions directly to error without fallback.
 */
export function useZeroCopyRetouchStage(
  options: UseZeroCopyRetouchStageOptions,
): ZeroCopyStageHandle {
  const canvasRef = ref<HTMLCanvasElement | null>(null)
  const histogramSample = shallowRef<ImageData | null>(null)
  const gpuSupported = isWebGLRetouchSupported()
  const textureReady = ref(false)
  const contextFailed = ref(false)

  let renderer: WebGLRetouchRenderer | null = null
  let drawQueued = false
  let lastHistogramAt = 0
  let restoredListener: (() => void) | null = null

  const isGpuActive = computed(
    () =>
      gpuSupported &&
      !contextFailed.value &&
      options.isAdjustMode.value &&
      canvasRef.value !== null &&
      textureReady.value &&
      renderer !== null &&
      !renderer.isContextLost(),
  )
  const stageStatus = computed<ZeroCopyStageStatus>(() => {
    if (!options.isAdjustMode.value) return 'idle'
    if (contextFailed.value || !gpuSupported) return 'error'
    if (isGpuActive.value) return 'ready'
    return 'idle'
  })

  function setCanvasRef(element: Element | ComponentPublicInstance | null): void {
    canvasRef.value = element instanceof HTMLCanvasElement ? element : null
  }

  function disposeRenderer(): void {
    renderer?.dispose()
    renderer = null
    textureReady.value = false
    drawQueued = false
  }

  function scheduleDraw(): void {
    if (drawQueued || !renderer || renderer.isContextLost()) return
    drawQueued = true
    const schedule = typeof requestAnimationFrame === 'function'
      ? requestAnimationFrame
      : (callback: () => void) => globalThis.setTimeout(callback, 16)
    schedule(() => {
      drawQueued = false
      if (!renderer || renderer.isContextLost()) return
      try {
        renderer.updateUniforms(
          bakeWebGLRetouchUniforms(compileImageRetouchPlan(effectLayers.value)),
        )
        renderer.render()
        maybeSampleHistogram()
      } catch {
        contextFailed.value = true
      }
    })
  }

  const effectLayers = computed(() => flattenVisibleRetouchEffectLayers(options.layers.value))

  function maybeSampleHistogram(): void {
    const now = Date.now()
    if (now - lastHistogramAt < HISTOGRAM_SAMPLE_THROTTLE_MS) return
    lastHistogramAt = now
    try {
      histogramSample.value = renderer?.readHistogramSample() ?? null
    } catch {
      histogramSample.value = null
    }
  }

  function uploadDomImage(image: HTMLImageElement): boolean {
    if (!renderer) return false
    try {
      renderer.updateSourceImage(image, image.naturalWidth, image.naturalHeight)
      textureReady.value = true
      contextFailed.value = false
      return true
    } catch {
      return false
    }
  }

  function bindSource(): void {
    textureReady.value = false
    if (!renderer || !options.isAdjustMode.value) return
    const domImage = options.resolveDomImage()
    if (domImage && uploadDomImage(domImage)) {
      scheduleDraw()
      return
    }
    contextFailed.value = true
  }

  function ensureRenderer(): void {
    const canvas = canvasRef.value
    if (!canvas || !options.isAdjustMode.value || !gpuSupported) return
    if (!renderer) {
      try {
        renderer = createWebGLRetouchRenderer(canvas)
      } catch {
        renderer = null
      }
      if (!renderer) {
        contextFailed.value = true
        return
      }
      attachRestoreListener(canvas)
    }
    if (renderer.isContextLost()) {
      contextFailed.value = true
      return
    }
    bindSource()
  }

  function attachRestoreListener(canvas: HTMLCanvasElement): void {
    if (restoredListener) return
    restoredListener = () => {
      // Textures die with the context: rebuild and rebind from scratch.
      disposeRenderer()
      contextFailed.value = false
      ensureRenderer()
    }
    canvas.addEventListener('webglcontextrestored', restoredListener)
  }

  // Single shallow watcher: layer identity changes flow straight into baked
  // uniforms + one coalesced draw. No deep watch, no array recreation here.
  watch(
    () => options.layers.value,
    () => {
      if (!isGpuActive.value) return
      scheduleDraw()
    },
  )
  watch([canvasRef, () => options.isAdjustMode.value], () => {
    if (!options.isAdjustMode.value) {
      textureReady.value = false
      drawQueued = false
      return
    }
    ensureRenderer()
  })

  onBeforeUnmount(() => {
    const canvas = canvasRef.value
    if (canvas && restoredListener) canvas.removeEventListener('webglcontextrestored', restoredListener)
    restoredListener = null
    disposeRenderer()
  })

  return {
    canvasRef,
    setCanvasRef,
    isGpuActive,
    stageStatus,
    histogramSample,
  }
}
