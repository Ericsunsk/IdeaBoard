import type {
  PreviewScene,
  PreviewSceneConnection,
  PreviewSceneMediaRef,
  PreviewSceneNode,
} from './scene'

// Pure Scene-to-thumbnail composition. SVG applies tokens and media delivery;
// this module owns geometry, display eligibility, and node paint order.

/** Fixed thumbnail delivery tier: independent of the canvas zoom quality. */
export const SCENE_THUMBNAIL_TIER = 'preview'

/** Maximum rasters per card in stable Scene order; remaining geometry stays visible. */
export const SCENE_THUMBNAIL_MAX_RASTERS_PER_CARD = 6

type SceneThumbnailRect = { x: number; y: number; width: number; height: number }

type SceneThumbnailRaster = {
  key: string
  assetId: string
  frame: SceneThumbnailRect
}

/** Unit-space viewBox (`0 0 extent`); null for empty boards (zero extent). */
function sceneViewBox(scene: PreviewScene): string | null {
  if (scene.nodes.length === 0) return null
  return `0 0 ${scene.extent.width} ${scene.extent.height}`
}

function mediaAssetIdOf(media: PreviewSceneMediaRef): string | null {
  const asset = media.asset.trim()
  // Numeric-only identities (board-local `asset_numeric` without an opaque
  // `asset` string) cannot address workspace delivery, so they resolve to no
  // raster — the frame renders as a placeholder, never a fabricated image.
  return asset ? asset : null
}

/**
 * Latest-output-wins display pick for an image node: the last `output` media entry, else the `source` entry.
 * Prompt and fusion frames never use this (prompts are simplified shapes;
 * fusion resolves its output and input slots in the composition).
 */
function pickSceneDisplayMedia(node: PreviewSceneNode): PreviewSceneMediaRef | null {
  if (node.kind !== 'image') return null
  let source: PreviewSceneMediaRef | null = null
  for (const media of node.media) {
    if (media.role === 'output') return media
    if (!source && media.role === 'source') source = media
  }
  return source
}

/**
 * Deterministic fusion slot grid in unit space: 1 centered, 2 side by side,
 * 3 as a triangle row, 4 as a 2x2 grid. Slots only carry input rasters when
 * the fusion node has no displayable output (see below).
 */
function fusionThumbnailSlotRect(
  frame: SceneThumbnailRect,
  index: number,
  count: number,
): SceneThumbnailRect {
  const slots = Math.max(1, Math.min(4, count))
  const inset = 0.12
  const inner = {
    x: frame.x + frame.width * inset,
    y: frame.y + frame.height * inset,
    width: frame.width * (1 - inset * 2),
    height: frame.height * (1 - inset * 2),
  }
  if (slots === 1) return inner
  if (slots === 2) {
    const half = inner.width / 2
    return index === 0
      ? { ...inner, width: half }
      : { x: inner.x + half, y: inner.y, width: inner.width - half, height: inner.height }
  }
  if (slots === 3) {
    const third = inner.width / 3
    return {
      x: inner.x + third * Math.min(2, index),
      y: inner.y,
      width: third,
      height: inner.height,
    }
  }
  const halfWidth = inner.width / 2
  const halfHeight = inner.height / 2
  return {
    x: inner.x + halfWidth * (index % 2),
    y: inner.y + halfHeight * (index < 2 ? 0 : 1),
    width: halfWidth,
    height: halfHeight,
  }
}

function frameOf(node: PreviewSceneNode): SceneThumbnailRect {
  return { x: node.x, y: node.y, width: node.width, height: node.height }
}

function rasterOf(
  node: PreviewSceneNode,
  media: PreviewSceneMediaRef,
  frame: SceneThumbnailRect,
): SceneThumbnailRaster | null {
  const assetId = mediaAssetIdOf(media)
  if (!assetId) return null
  return {
    key: `${node.id}-${media.role}-${media.index}`,
    assetId,
    frame,
  }
}

/**
 * Right-edge-center → left-edge-center cubic in unit space. The control
 * offset is extent-relative (never pixels) so the same scene draws the same
 * curve on every device and pixel ratio.
 */
function sceneConnectionPath(connection: PreviewSceneConnection): string {
  const { start, end } = connection
  const curve = Math.max(0.05, Math.abs(end.x - start.x) * 0.42)
  return (
    `M ${start.x} ${start.y} ` +
    `C ${start.x + curve} ${start.y}, ${end.x - curve} ${end.y}, ${end.x} ${end.y}`
  )
}

function cornerRadius(rect: SceneThumbnailRect): number {
  return Math.min(0.02, rect.width * 0.05, rect.height * 0.05)
}

function imageGlyphTransform(rect: SceneThumbnailRect): string {
  const size = Math.min(rect.width, rect.height) * 0.5
  return `translate(${rect.x + rect.width / 2} ${rect.y + rect.height / 2}) scale(${size})`
}

function promptBars(node: PreviewSceneNode): SceneThumbnailRect[] {
  const padding = node.width * 0.1
  const height = Math.max(0.008, node.height * 0.035)
  const step = (node.height - padding * 2) / 4
  return [0.76, 0.62, 0.68, 0.42].map((fraction, index) => ({
    x: node.x + padding,
    y: node.y + padding + index * step,
    width: Math.max(0.01, node.width * fraction - padding * 2),
    height,
  }))
}

function composeFusion(node: PreviewSceneNode) {
  if (node.kind !== 'fusion')
    return { output: undefined, inputs: [], slots: [], blend: null, spokes: [] }
  const frame = frameOf(node)
  const output = node.media.find((media) => media.role === 'output' && mediaAssetIdOf(media))
  const inputs = output ? [] : node.media.filter((media) => media.role === 'input').slice(0, 4)
  const slots = !output
    ? Array.from({ length: inputs.length || 2 }, (_, index) => {
        const slot = fusionThumbnailSlotRect(frame, index, inputs.length || 2)
        return { ...slot, radius: cornerRadius(slot), glyphTransform: imageGlyphTransform(slot) }
      })
    : []
  const blend =
    slots.length > 0
      ? {
          x: node.x + (node.blend?.x ?? 0.5) * node.width,
          y: node.y + (node.blend?.y ?? 0.5) * node.height,
        }
      : null
  return {
    output,
    inputs,
    slots,
    blend,
    spokes: blend
      ? slots.map((slot) => ({
          x1: blend.x,
          y1: blend.y,
          x2: slot.x + slot.width / 2,
          y2: slot.y + slot.height / 2,
        }))
      : [],
  }
}

function composeNode(node: PreviewSceneNode, remainingRasters: number) {
  const frame = frameOf(node)
  const fusion = composeFusion(node)
  const rasters: SceneThumbnailRaster[] = []
  const display = node.kind === 'image' ? pickSceneDisplayMedia(node) : fusion.output
  if (display && remainingRasters > 0) {
    const raster = rasterOf(node, display, frame)
    if (raster) rasters.push(raster)
  } else {
    fusion.inputs.forEach((media, index) => {
      if (rasters.length >= remainingRasters) return
      const raster = rasterOf(node, media, fusion.slots[index]!)
      if (raster) rasters.push(raster)
    })
  }
  return {
    id: node.id,
    kind: node.kind,
    ...frame,
    radius: cornerRadius(frame),
    bars: node.kind === 'prompt' ? promptBars(node) : [],
    slots: fusion.slots,
    blend: fusion.blend,
    spokes: fusion.spokes,
    glyphTransform: node.kind === 'image' || fusion.output ? imageGlyphTransform(frame) : null,
    rasters: rasters.map((raster) => ({ ...raster, radius: cornerRadius(raster.frame) })),
  }
}

/** Complete presentation in Scene node paint order; capped raster reads preserve all geometry. */
export function composeSceneThumbnail(scene: PreviewScene) {
  let remaining = SCENE_THUMBNAIL_MAX_RASTERS_PER_CARD
  const nodes = scene.nodes.map((node) => {
    const presentation = composeNode(node, remaining)
    remaining -= presentation.rasters.length
    return presentation
  })
  return {
    viewBox: sceneViewBox(scene),
    groups: scene.groups.map((group) => ({ ...group, radius: cornerRadius(group) })),
    edges: scene.connections.map((connection) => ({
      key: connection.key,
      d: sceneConnectionPath(connection),
    })),
    nodes,
  }
}
