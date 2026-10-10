import type { ComputedRef, Ref } from 'vue'
import type { RetouchEffectLayer, RetouchLayer } from '../core/model.ts'
import {
  resolveDomTextureImage,
  useZeroCopyRetouchStage,
  type ZeroCopyStageHandle,
} from './stage/useZeroCopyRetouchStage.ts'
import { useStableSourceImage } from './histogram/stableSourceImage.ts'
import { useRetouchHistogramSource } from './histogram/useRetouchHistogramSource'

/**
 * Opaque editor-facing layer stack. image-tools reads chrome editor layers
 * through this barrel-owned alias instead of naming the concrete retouch
 * layer union, so the union stays private to retouch.
 */
export type RetouchEditorLayers = RetouchLayer[]

export type RetouchChromeSourceOptions = {
  isAdjustMode: Ref<boolean>
  layers: Ref<readonly RetouchLayer[]>
  selectedEffectLayer: ComputedRef<RetouchEffectLayer | null>
  /** Board-owned stage element lookup; DOM sampling stays inside retouch. */
  getStageElement: () => Element | null
  /** Panel session identity (node id, '' when there is no node). */
  sessionKey: Ref<string> | ComputedRef<string>
  /*
   * Explicit pending slot forwarded to the histogram resolver. Optional so
   * existing callers keep working: without it an unknown target id still
   * resolves to the raw source (legacy id-miss path).
   */
  pendingEffectLayer?:
    Ref<RetouchEffectLayer | null> | ComputedRef<RetouchEffectLayer | null> | undefined
}

export type RetouchChromeSource = {
  retouchStage: ZeroCopyStageHandle
  retouchHistogramSource: ComputedRef<ImageData | null>
}

/**
 * Retouch-owned chrome source: zero-copy GPU stage + stable histogram base.
 * image-tools supplies only reactive session reads and the board stage
 * lookup; all DOM sampling and histogram resolution live here.
 */
export function createRetouchChromeSource(
  options: RetouchChromeSourceOptions,
): RetouchChromeSource {
  // Zero-copy GPU stage (issues #672, #673): the mounted canvas IS the render
  // target. Texture binding uses the DOM image already on screen directly.
  const retouchStage = useZeroCopyRetouchStage({
    isAdjustMode: options.isAdjustMode,
    layers: options.layers,
    resolveDomImage: () => resolveStageDomImage(),
  })

  // Shared DOM source: the raw on-screen node image, before any GPU effect.
  function resolveStageDomImage(): HTMLImageElement | null {
    const stageElement = options.getStageElement()
    return (
      resolveDomTextureImage(retouchStage.canvasRef.value?.parentElement ?? null) ??
      resolveDomTextureImage(stageElement)
    )
  }

  // Histogram input is the stable pre-effect source (Lightroom semantics):
  // the mountain shows the selected layer's input and never moves while its
  // own draft params are dragged. Upstream layers still recompute through
  // the histogram source resolver below.
  const { stablePreviewSource } = useStableSourceImage({
    isAdjustMode: options.isAdjustMode,
    resolveDomImage: () => resolveStageDomImage(),
    refreshSource: options.layers,
    // Freeze the mountain per panel session: mount and first-drag DOM picks
    // may resolve different imgs, which jittered the peak once per session.
    sessionKey: options.sessionKey,
  })
  const { retouchHistogramSource } = useRetouchHistogramSource({
    isAdjustMode: options.isAdjustMode,
    layers: options.layers,
    selectedEffectLayer: options.selectedEffectLayer,
    previewSource: stablePreviewSource,
    pendingEffectLayer: options.pendingEffectLayer,
  })

  return {
    retouchStage,
    retouchHistogramSource,
  }
}
