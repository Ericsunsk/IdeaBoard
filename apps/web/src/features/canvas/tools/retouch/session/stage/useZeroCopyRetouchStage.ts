import { computed, onBeforeUnmount, ref, watch, type ComputedRef, type Ref } from 'vue'
import { compileImageRetouchPlan } from '../../core/engine.ts'
import {
  bakeWebGLRetouchUniforms,
  createWebGLRetouchRenderer,
  isWebGLRetouchSupported,
  type WebGLRetouchRenderer,
} from './runtime/webglEngine.ts'
import { flattenVisibleRetouchEffectLayers } from '../../core/layers/layerEffects.ts'
import type { RetouchLayer } from '../../core/model.ts'

export type ZeroCopyStageStatus = 'ready' | 'error' | 'idle'

export type ZeroCopyStageHandle = {
  /** Bound by the stage presentation canvas. The GPU draws here directly. */
  canvasRef: Ref<HTMLCanvasElement | null>
  setCanvasRef: (element: Element | ComponentPublicInstance | null) => void
  isGpuActive: ComputedRef<boolean>
  stageStatus: ComputedRef<ZeroCopyStageStatus>
}

type ComponentPublicInstance = { $el?: unknown }

type UseZeroCopyRetouchStageOptions = {
  isAdjustMode: Ref<boolean>
  layers: Ref<readonly RetouchLayer[]>
  /** DOM texture source: the already-rendered node image on screen. */
  resolveDomImage: () => HTMLImageElement | null
}

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
export function resolveDomTextureImage(
  stageElement: Element | null | undefined,
): HTMLImageElement | null {
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
  const gpuSupported = isWebGLRetouchSupported()
  const textureReady = ref(false)
  const contextFailed = ref(false)

  let renderer: WebGLRetouchRenderer | null = null
  let boundCanvas: HTMLCanvasElement | null = null
  let drawQueued = false
  let drawHandle: number | null = null
  let restoredListener: (() => void) | null = null
  // Set when bind found no usable DOM image yet (e.g. entering Retouch
  // before the node image decodes). Unlike contextFailed it is recoverable:
  // the next layer change or canvas/mode transition retries the bind.
  let sourcePending = false

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

  function cancelScheduledDraw(): void {
    if (drawHandle === null) {
      drawQueued = false
      return
    }
    const handle = drawHandle
    drawHandle = null
    drawQueued = false
    try {
      if (typeof globalThis.cancelAnimationFrame === 'function') {
        globalThis.cancelAnimationFrame(handle)
      }
    } catch {
      // Ignore cancellation errors during teardown.
    }
  }

  function disposeRenderer(): void {
    cancelScheduledDraw()
    const canvas = boundCanvas
    boundCanvas = null
    if (canvas && restoredListener)
      canvas.removeEventListener('webglcontextrestored', restoredListener)
    restoredListener = null
    renderer?.dispose()
    renderer = null
    textureReady.value = false
    sourcePending = false
  }

  function scheduleDraw(): void {
    if (drawQueued || !renderer || renderer.isContextLost()) return
    drawQueued = true
    const onDraw = () => {
      drawHandle = null
      drawQueued = false
      if (!renderer || renderer.isContextLost()) return
      try {
        renderer.updateUniforms(
          bakeWebGLRetouchUniforms(compileImageRetouchPlan(effectLayers.value)),
        )
        renderer.render()
      } catch {
        contextFailed.value = true
      }
    }
    if (typeof globalThis.requestAnimationFrame !== 'function') {
      drawQueued = false
      return
    }
    drawHandle = globalThis.requestAnimationFrame(onDraw)
  }

  const effectLayers = computed(() => flattenVisibleRetouchEffectLayers(options.layers.value))

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
      sourcePending = false
      scheduleDraw()
      return
    }
    if (!domImage) {
      // The stage DOM may still be mounting or decoding (e.g. entering
      // Retouch right after the node image starts loading). Stay pending and
      // retry on the next layer change or canvas/mode transition instead of
      // failing the whole session permanently.
      sourcePending = true
      return
    }
    contextFailed.value = true
  }

  function ensureRenderer(): void {
    const canvas = canvasRef.value
    if (!canvas || !options.isAdjustMode.value || !gpuSupported) return
    if (renderer && boundCanvas !== canvas) {
      // The stage canvas was replaced (node switch / remount). A GL renderer
      // draws into its original canvas, so rebuild it instead of rendering
      // into a detached surface while the visible stage stays blank.
      disposeRenderer()
    }
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
      boundCanvas = canvas
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
  // A pending source bind also retries here, so the first slider drag after
  // an early entry rebinds from the now-decoded DOM image and draws.
  watch(
    () => options.layers.value,
    () => {
      if (isGpuActive.value) {
        scheduleDraw()
        return
      }
      if (sourcePending && options.isAdjustMode.value && canvasRef.value) {
        ensureRenderer()
      }
    },
  )
  watch([canvasRef, () => options.isAdjustMode.value, readSourceKey], () => {
    if (!options.isAdjustMode.value) {
      textureReady.value = false
      cancelScheduledDraw()
      return
    }
    ensureRenderer()
  })

  function readSourceKey(): string {
    const image = options.resolveDomImage()
    return image
      ? `${image.currentSrc || image.src} ${image.naturalWidth}x${image.naturalHeight}`
      : ''
  }

  onBeforeUnmount(() => {
    const canvas = canvasRef.value
    if (canvas && restoredListener)
      canvas.removeEventListener('webglcontextrestored', restoredListener)
    restoredListener = null
    disposeRenderer()
  })

  return {
    canvasRef,
    setCanvasRef,
    isGpuActive,
    stageStatus,
  }
}
