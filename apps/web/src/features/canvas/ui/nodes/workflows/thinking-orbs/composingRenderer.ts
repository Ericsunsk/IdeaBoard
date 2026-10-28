/**
 * Adapted from thinking-orbs at commit e94f207ea122f8cca0aaa6409ab7fe82d55c38f1.
 * Original source: src/engine/core.ts, src/engine/ribbon.ts, src/presets.ts.
 * Copyright (c) 2026 Jakub Antalik. Licensed under MIT; see LICENSE.txt.
 */

type Dot = {
  x: number
  y: number
  z: number
  r: number
  white: number
  alpha: number
}

type Projector = (dot: Dot, x: number, y: number, z: number) => void

const composingOptions = {
  bandMultiplier: 3.9,
  ghostCount: 38,
  lanes: 3,
  radiusBase: 0.935,
  radiusDepth: 1.445,
  radiusMinimum: 0.3,
  radiusScalePower: 0.6,
  segments: 44,
  spin: 0,
  wobbleMultiplier: 1,
} as const

export const COMPOSING_ORB_BASE_SPEED = 2.34

function fibonacciDirection(index: number, count: number): [number, number, number] {
  const goldenAngle = Math.PI * (3 - Math.sqrt(5))
  const y = 1 - (2 * (index + 0.5)) / count
  const radialDistance = Math.sqrt(1 - y * y)
  const angle = index * goldenAngle
  return [radialDistance * Math.cos(angle), y, radialDistance * Math.sin(angle)]
}

const ghostDirections = Array.from({ length: composingOptions.ghostCount }, (_, index) =>
  fibonacciDirection(index, composingOptions.ghostCount),
)
const segmentDirections = Array.from({ length: composingOptions.segments }, (_, index) => {
  const angle = (index / composingOptions.segments) * 2 * Math.PI
  return { angle, x: Math.cos(angle), y: Math.sin(angle) }
})
const laneCount = Math.max(1, Math.round(composingOptions.lanes * composingOptions.bandMultiplier))
const dotBuffers = new WeakMap<CanvasRenderingContext2D, { dots: Dot[]; paintOrder: Dot[] }>()

function getDotBuffer(context: CanvasRenderingContext2D) {
  let buffer = dotBuffers.get(context)
  if (!buffer) {
    const dots = Array.from(
      { length: composingOptions.ghostCount + laneCount * composingOptions.segments },
      () => ({ x: 0, y: 0, z: 0, r: 0, white: 0, alpha: 0 }),
    )
    buffer = { dots, paintOrder: [...dots] }
    dotBuffers.set(context, buffer)
  }
  return buffer
}

function createProjector(yaw: number, tilt: number, centerX: number, centerY: number): Projector {
  const sinTilt = Math.sin(tilt)
  const cosTilt = Math.cos(tilt)
  const sinYaw = Math.sin(yaw)
  const cosYaw = Math.cos(yaw)

  return (dot, x, y, z) => {
    const rotatedX = x * cosYaw + z * sinYaw
    const rotatedZ = -x * sinYaw + z * cosYaw
    const rotatedY = y * cosTilt - rotatedZ * sinTilt
    const depth = y * sinTilt + rotatedZ * cosTilt
    dot.x = centerX + rotatedX
    dot.y = centerY - rotatedY
    dot.z = depth
  }
}

function resolveRadiusScale(size: number): number {
  return (size / 300) ** composingOptions.radiusScalePower
}

function paintDots(context: CanvasRenderingContext2D, dots: Dot[], isDark: boolean) {
  dots.sort((left, right) => left.z - right.z)

  for (const dot of dots) {
    if (dot.alpha < 0.02) continue
    const ink = Math.min(1, Math.max(0, dot.white))
    const gray = Math.round((isDark ? 1 - ink : ink) * 255)
    context.fillStyle = `rgba(${gray},${gray},${gray},${dot.alpha})`
    context.beginPath()
    context.arc(dot.x, dot.y, Math.max(composingOptions.radiusMinimum, dot.r), 0, Math.PI * 2)
    context.fill()
  }
}

export function drawComposingOrb(
  context: CanvasRenderingContext2D,
  size: number,
  timeSeconds: number,
  isDark: boolean,
) {
  const centerX = size / 2
  const centerY = size / 2
  const orbRadius = (size / 2) * 0.78
  const cameraTilt = 0.3
  const projector = createProjector(
    timeSeconds * 0.1 * composingOptions.spin,
    cameraTilt,
    centerX,
    centerY,
  )
  const radiusScale = resolveRadiusScale(size)
  const { dots, paintOrder } = getDotBuffer(context)

  for (let index = 0; index < composingOptions.ghostCount; index += 1) {
    const dot = dots[index]!
    const direction = ghostDirections[index]!
    projector(dot, direction[0] * orbRadius, direction[1] * orbRadius, direction[2] * orbRadius)
    const depth = (dot.z / orbRadius + 1) / 2
    dot.r = 0.8 * radiusScale
    dot.white = 0.78
    dot.alpha = 0.1 + 0.22 * depth
  }

  const rotation = timeSeconds * 0.24 * composingOptions.spin
  const tilt = 0.55 + 0.3 * Math.sin(timeSeconds * 0.18) * composingOptions.spin
  const axisX = Math.cos(rotation)
  const axisZ = Math.sin(rotation)
  const crossX = -axisZ * Math.sin(tilt)
  const crossY = Math.cos(tilt)
  const crossZ = axisX * Math.sin(tilt)
  const normalX = -axisZ * crossY
  const normalY = axisZ * crossX - axisX * crossZ
  const normalZ = axisX * crossY

  for (let lane = 0; lane < laneCount; lane += 1) {
    const laneOffset = (lane - (laneCount - 1) / 2) * 0.075
    const edgeDistance = Math.abs(lane - (laneCount - 1) / 2) / Math.max(1, (laneCount - 1) / 2)

    for (let segment = 0; segment < composingOptions.segments; segment += 1) {
      const dot = dots[composingOptions.ghostCount + lane * composingOptions.segments + segment]!
      const { angle, x: segmentX, y: segmentY } = segmentDirections[segment]!
      const wobble =
        (0.16 * Math.sin(angle * 3 - timeSeconds * 1.7 + lane * 0.22) +
          0.07 * Math.sin(angle * 5 + timeSeconds * 1.1)) *
        composingOptions.wobbleMultiplier
      const offset = laneOffset + wobble
      const x = axisX * segmentX + crossX * segmentY + normalX * offset
      const y = crossY * segmentY + normalY * offset
      const z = axisZ * segmentX + crossZ * segmentY + normalZ * offset
      const length = Math.sqrt(x * x + y * y + z * z)
      projector(dot, (x / length) * orbRadius, (y / length) * orbRadius, (z / length) * orbRadius)
      const depth = (dot.z / orbRadius + 1) / 2
      dot.r =
        (composingOptions.radiusBase + composingOptions.radiusDepth * depth) *
        (1 - 0.25 * edgeDistance) *
        radiusScale
      dot.white = 0.52 - 0.44 * depth + 0.18 * edgeDistance
      dot.alpha = 0.4 + 0.6 * depth
    }
  }

  for (let index = 0; index < dots.length; index += 1) paintOrder[index] = dots[index]!
  paintDots(context, paintOrder, isDark)
}
