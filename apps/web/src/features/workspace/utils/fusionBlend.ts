export interface FusionBlendPoint {
  x: number
  y: number
}

export const DEFAULT_FUSION_BLEND_POINT: FusionBlendPoint = { x: 0.5, y: 0.5 }
export const FUSION_SLOT_LAYOUT = {
  maxSlots: 4,
  inset: 20,
  gap: 6,
  minSize: 46,
  maxSize: 58,
  fallbackStageWidth: 286,
} as const

type PointTuple = [number, number]
type FusionSlotPosition =
  | 'center'
  | 'top-left'
  | 'top-right'
  | 'top-center'
  | 'bottom-left'
  | 'bottom-right'

const FUSION_SLOT_POSITION_TABLE: Record<number, readonly FusionSlotPosition[]> = {
  1: ['center', 'top-left', 'top-right', 'bottom-right'],
  2: ['top-left', 'bottom-right', 'top-right', 'bottom-left'],
  3: ['top-center', 'bottom-left', 'bottom-right', 'top-left'],
  4: ['top-left', 'top-right', 'bottom-left', 'bottom-right'],
}

const FUSION_SLOT_ANCHOR_FACTORIES: Record<FusionSlotPosition, (geometry: ReturnType<typeof getFusionSlotGeometry>) => FusionBlendPoint> = {
  center: () => ({ ...DEFAULT_FUSION_BLEND_POINT }),
  'top-left': (geometry) => ({ x: geometry.centerNear, y: geometry.centerNear }),
  'top-right': (geometry) => ({ x: geometry.centerFar, y: geometry.centerNear }),
  'top-center': (geometry) => ({ x: 0.5, y: geometry.centerNear }),
  'bottom-left': (geometry) => ({ x: geometry.centerNear, y: geometry.centerFar }),
  'bottom-right': (geometry) => ({ x: geometry.centerFar, y: geometry.centerFar }),
}

function clamp01(value: number): number {
  return Math.max(0, Math.min(1, value))
}

function resolveStageWidth(stageWidth?: number): number {
  const value = Number(stageWidth)
  return Number.isFinite(value) && value > 0 ? value : FUSION_SLOT_LAYOUT.fallbackStageWidth
}

function resolveSlotSize(stageWidth?: number): number {
  const width = resolveStageWidth(stageWidth)
  const rawSize =
    (width - FUSION_SLOT_LAYOUT.inset * 2 - FUSION_SLOT_LAYOUT.gap * (FUSION_SLOT_LAYOUT.maxSlots - 1)) /
    FUSION_SLOT_LAYOUT.maxSlots
  return Math.max(FUSION_SLOT_LAYOUT.minSize, Math.min(rawSize, FUSION_SLOT_LAYOUT.maxSize))
}

function getFusionSlotGeometry(stageWidth?: number) {
  const width = resolveStageWidth(stageWidth)
  const slotSize = resolveSlotSize(width)
  const centerNear = (FUSION_SLOT_LAYOUT.inset + slotSize / 2) / width
  const innerNear = (FUSION_SLOT_LAYOUT.inset + slotSize) / width
  return {
    centerNear,
    centerFar: 1 - centerNear,
    innerNear,
    innerFar: 1 - innerNear,
  }
}

function getFusionSlotPosition(imageCount: number, index: number): FusionSlotPosition {
  const count = Math.max(0, Math.min(Math.round(imageCount), FUSION_SLOT_LAYOUT.maxSlots))
  const slots = FUSION_SLOT_POSITION_TABLE[count] ?? FUSION_SLOT_POSITION_TABLE[4]
  return slots[index] ?? 'center'
}

function getAnchorForPosition(
  position: FusionSlotPosition,
  stageWidth?: number,
): FusionBlendPoint {
  const geometry = getFusionSlotGeometry(stageWidth)
  return FUSION_SLOT_ANCHOR_FACTORIES[position](geometry)
}

export function getFusionSlotAnchor(
  imageCount: number,
  index: number,
  stageWidth?: number,
): FusionBlendPoint {
  return getAnchorForPosition(getFusionSlotPosition(imageCount, index), stageWidth)
}

function getFusionBlendArea(imageCount: number, stageWidth?: number): FusionBlendPoint[] {
  const count = Math.max(0, Math.min(Math.round(imageCount), FUSION_SLOT_LAYOUT.maxSlots))
  const geometry = getFusionSlotGeometry(stageWidth)
  if (count <= 1) return [{ ...DEFAULT_FUSION_BLEND_POINT }]
  if (count === 2) {
    return [
      { x: geometry.innerNear, y: geometry.innerNear },
      { x: geometry.innerFar, y: geometry.innerFar },
    ]
  }
  if (count === 3) {
    return [
      { x: 0.5, y: geometry.innerNear },
      { x: geometry.innerNear, y: geometry.innerFar },
      { x: geometry.innerFar, y: geometry.innerFar },
    ]
  }
  return [
    { x: geometry.innerNear, y: geometry.innerNear },
    { x: geometry.innerFar, y: geometry.innerNear },
    { x: geometry.innerNear, y: geometry.innerFar },
    { x: geometry.innerFar, y: geometry.innerFar },
  ]
}

function dot(ax: number, ay: number, bx: number, by: number) {
  return ax * bx + ay * by
}

function subtract(a: FusionBlendPoint, b: FusionBlendPoint): PointTuple {
  return [a.x - b.x, a.y - b.y]
}

function scale(v: PointTuple, t: number): PointTuple {
  return [v[0] * t, v[1] * t]
}

function add(a: FusionBlendPoint, v: PointTuple): FusionBlendPoint {
  return { x: a.x + v[0], y: a.y + v[1] }
}

function normalizeWeights(weights: number[]): number[] {
  const nonNegative = weights.map((value) => (Number.isFinite(value) ? Math.max(0, value) : 0))
  const sum = nonNegative.reduce((acc, value) => acc + value, 0)
  if (sum <= 0) {
    const fallback = nonNegative.length > 0 ? 1 / nonNegative.length : 0
    return nonNegative.map(() => fallback)
  }
  return nonNegative.map((value) => value / sum)
}

function projectPointToSegment(
  point: FusionBlendPoint,
  start: FusionBlendPoint,
  end: FusionBlendPoint,
): { point: FusionBlendPoint; t: number } {
  const [vx, vy] = subtract(end, start)
  const [wx, wy] = subtract(point, start)
  const denom = dot(vx, vy, vx, vy)
  if (denom <= 0) {
    return { point: { ...start }, t: 0 }
  }
  const t = clamp01(dot(wx, wy, vx, vy) / denom)
  return {
    point: add(start, scale([vx, vy], t)),
    t,
  }
}

function barycentricWeights(
  point: FusionBlendPoint,
  a: FusionBlendPoint,
  b: FusionBlendPoint,
  c: FusionBlendPoint,
): [number, number, number] {
  const v0 = subtract(b, a)
  const v1 = subtract(c, a)
  const v2 = subtract(point, a)
  const d00 = dot(v0[0], v0[1], v0[0], v0[1])
  const d01 = dot(v0[0], v0[1], v1[0], v1[1])
  const d11 = dot(v1[0], v1[1], v1[0], v1[1])
  const d20 = dot(v2[0], v2[1], v0[0], v0[1])
  const d21 = dot(v2[0], v2[1], v1[0], v1[1])
  const denom = d00 * d11 - d01 * d01
  if (Math.abs(denom) < 1e-8) return [1 / 3, 1 / 3, 1 / 3]
  const v = (d11 * d20 - d01 * d21) / denom
  const w = (d00 * d21 - d01 * d20) / denom
  const u = 1 - v - w
  return [u, v, w]
}

function isPointInsideTriangle(weights: [number, number, number]): boolean {
  return weights[0] >= 0 && weights[1] >= 0 && weights[2] >= 0
}

function closestPointOnTriangle(
  point: FusionBlendPoint,
  a: FusionBlendPoint,
  b: FusionBlendPoint,
  c: FusionBlendPoint,
): FusionBlendPoint {
  const rawWeights = barycentricWeights(point, a, b, c)
  if (isPointInsideTriangle(rawWeights)) return point

  const projections = [
    projectPointToSegment(point, a, b).point,
    projectPointToSegment(point, b, c).point,
    projectPointToSegment(point, c, a).point,
  ]
  let nearest = projections[0]
  let nearestDistance = Number.POSITIVE_INFINITY
  for (const projection of projections) {
    const dx = projection.x - point.x
    const dy = projection.y - point.y
    const distance = dx * dx + dy * dy
    if (distance < nearestDistance) {
      nearestDistance = distance
      nearest = projection
    }
  }
  return nearest
}

export function normalizeFusionBlendPoint(value: unknown): FusionBlendPoint {
  const candidate = value as Partial<FusionBlendPoint> | null | undefined
  const x = Number(candidate?.x)
  const y = Number(candidate?.y)
  return {
    x: Number.isFinite(x) ? clamp01(x) : DEFAULT_FUSION_BLEND_POINT.x,
    y: Number.isFinite(y) ? clamp01(y) : DEFAULT_FUSION_BLEND_POINT.y,
  }
}

export function areFusionBlendPointsEqual(
  left: FusionBlendPoint,
  right: FusionBlendPoint,
  epsilon = 1e-6,
): boolean {
  return Math.abs(left.x - right.x) < epsilon && Math.abs(left.y - right.y) < epsilon
}

export function getFusionSlotAnchors(imageCount: number, stageWidth?: number): FusionBlendPoint[] {
  const count = Math.max(0, Math.min(Math.round(imageCount), FUSION_SLOT_LAYOUT.maxSlots))
  return Array.from({ length: count }, (_, index) => getFusionSlotAnchor(count, index, stageWidth))
}

export function getFusionLineAnchors(imageCount: number, stageWidth?: number): FusionBlendPoint[] {
  const count = Math.max(0, Math.min(Math.round(imageCount), FUSION_SLOT_LAYOUT.maxSlots))
  return getFusionBlendArea(count, stageWidth).slice(0, count)
}

export function getDefaultBlendPointForImageCount(
  imageCount: number,
  stageWidth?: number,
): FusionBlendPoint {
  if (imageCount <= 1) return { ...DEFAULT_FUSION_BLEND_POINT }
  if (imageCount === 2) {
    const area = getFusionBlendArea(imageCount, stageWidth)
    const start = area[0]!
    const end = area[1]!
    return { x: (start.x + end.x) / 2, y: (start.y + end.y) / 2 }
  }
  if (imageCount === 3) {
    const area = getFusionBlendArea(imageCount, stageWidth)
    const a = area[0]!
    const b = area[1]!
    const c = area[2]!
    return {
      x: (a.x + b.x + c.x) / 3,
      y: (a.y + b.y + c.y) / 3,
    }
  }
  return { x: 0.5, y: 0.5 }
}

export function constrainFusionBlendPoint(
  value: unknown,
  imageCount: number,
  stageWidth?: number,
): FusionBlendPoint {
  const point = normalizeFusionBlendPoint(value)
  if (imageCount <= 1) return getDefaultBlendPointForImageCount(imageCount, stageWidth)

  if (imageCount === 2) {
    const area = getFusionBlendArea(imageCount, stageWidth)
    const start = area[0]!
    const end = area[1]!
    return projectPointToSegment(point, start, end).point
  }

  if (imageCount === 3) {
    const area = getFusionBlendArea(imageCount, stageWidth)
    const a = area[0]!
    const b = area[1]!
    const c = area[2]!
    return closestPointOnTriangle(point, a, b, c)
  }

  const quad = getFusionBlendArea(imageCount, stageWidth)
  const minX = quad[0]!.x
  const maxX = quad[1]!.x
  const minY = quad[0]!.y
  const maxY = quad[2]!.y
  return {
    x: Math.max(minX, Math.min(maxX, point.x)),
    y: Math.max(minY, Math.min(maxY, point.y)),
  }
}

export function computeFusionBlendWeights(
  imageCount: number,
  rawBlendPoint: unknown,
  stageWidth?: number,
): number[] {
  if (imageCount <= 0) return []
  if (imageCount === 1) return [1]

  const blendPoint = constrainFusionBlendPoint(rawBlendPoint, imageCount, stageWidth)

  if (imageCount === 2) {
    const area = getFusionBlendArea(imageCount, stageWidth)
    const start = area[0]!
    const end = area[1]!
    const projected = projectPointToSegment(blendPoint, start, end)
    return normalizeWeights([1 - projected.t, projected.t])
  }

  if (imageCount === 3) {
    const area = getFusionBlendArea(imageCount, stageWidth)
    const a = area[0]!
    const b = area[1]!
    const c = area[2]!
    const closest = closestPointOnTriangle(blendPoint, a, b, c)
    const weights = barycentricWeights(closest, a, b, c)
    return normalizeWeights([weights[0], weights[1], weights[2]])
  }

  const quad = getFusionBlendArea(imageCount, stageWidth)
  const minX = quad[0]!.x
  const maxX = quad[1]!.x
  const minY = quad[0]!.y
  const maxY = quad[2]!.y
  const width = maxX - minX
  const height = maxY - minY
  const u = width > 0 ? clamp01((blendPoint.x - minX) / width) : 0.5
  const v = height > 0 ? clamp01((blendPoint.y - minY) / height) : 0.5
  return normalizeWeights([(1 - u) * (1 - v), u * (1 - v), (1 - u) * v, u * v])
}

export function buildFusionDisplayPercentages(weights: number[]): number[] {
  if (weights.length === 0) return []
  const rawPercents = normalizeWeights(weights).map((value, index) => ({
    index,
    rounded: Math.round(value * 100),
    fraction: value * 100 - Math.floor(value * 100),
  }))
  let sum = rawPercents.reduce((acc, item) => acc + item.rounded, 0)
  if (sum !== 100) {
    const diff = 100 - sum
    const sorted = [...rawPercents].sort((left, right) =>
      diff > 0 ? right.fraction - left.fraction : left.fraction - right.fraction,
    )
    const step = diff > 0 ? 1 : -1
    for (let index = 0; index < Math.abs(diff); index += 1) {
      const target = sorted[index % sorted.length]
      target.rounded += step
    }
    sum = rawPercents.reduce((acc, item) => acc + item.rounded, 0)
    if (sum !== 100 && rawPercents.length > 0) {
      rawPercents[rawPercents.length - 1]!.rounded += 100 - sum
    }
  }
  return rawPercents.sort((left, right) => left.index - right.index).map((item) => item.rounded)
}
