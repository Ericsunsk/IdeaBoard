export type CanvasHandleProximityDirection = 'input' | 'output'

export type CanvasHandleProximityPoint = {
  x: number
  y: number
}

export type CanvasHandleProximityGeometry = {
  /**
   * Captures the shared Vue Flow surface geometry once for an evaluation.
   * Handle registrations must never read their own DOM rectangles from the pointer hot path.
   */
  prepare: () => void
  resolveAnchor: (
    nodeId: string,
    direction: CanvasHandleProximityDirection,
  ) => CanvasHandleProximityPoint | null
}

type CanvasHandleProximityState = {
  isActive: boolean
  point: CanvasHandleProximityPoint
}

type CanvasHandleProximityRegistration = {
  nodeId: string
  direction: CanvasHandleProximityDirection
  isEnabled: () => boolean
  onStateChange: (state: CanvasHandleProximityState) => void
}

type RegisteredHandle = CanvasHandleProximityRegistration & CanvasHandleProximityState

type Candidate = {
  registration: RegisteredHandle
  distance: number
  point: CanvasHandleProximityPoint
}

const PROXIMITY_RADIUS = 56
const SECTOR_COSINE = 0.5
const WINNER_EPSILON = 0.5
const ORIGIN: CanvasHandleProximityPoint = { x: 0, y: 0 }

function readCandidate(
  registration: RegisteredHandle,
  pointer: CanvasHandleProximityPoint,
  geometry: CanvasHandleProximityGeometry,
): Candidate | null {
  if (!registration.isEnabled()) return null
  const anchor = geometry.resolveAnchor(registration.nodeId, registration.direction)
  if (!anchor) return null

  const point = {
    x: pointer.x - anchor.x,
    y: pointer.y - anchor.y,
  }
  const distance = Math.hypot(point.x, point.y)
  if (distance > PROXIMITY_RADIUS) return null

  const outwardDirection = registration.direction === 'output' ? 1 : -1
  if (distance > 0 && (point.x * outwardDirection) / distance < SECTOR_COSINE) return null

  return { registration, distance, point }
}

function notify(registration: RegisteredHandle, state: CanvasHandleProximityState) {
  if (
    registration.isActive === state.isActive &&
    registration.point.x === state.point.x &&
    registration.point.y === state.point.y
  ) {
    return
  }
  registration.isActive = state.isActive
  registration.point = state.point
  registration.onStateChange(state)
}

export function createCanvasHandleProximityCoordinator(geometry: CanvasHandleProximityGeometry) {
  const registrations = new Set<RegisteredHandle>()
  let activeRegistration: RegisteredHandle | null = null
  let lastPointer: CanvasHandleProximityPoint | null = null
  let isSuspended = false

  function selectWinningCandidate(pointer: CanvasHandleProximityPoint | null) {
    let winner: Candidate | null = null

    if (pointer && !isSuspended) {
      geometry.prepare()
      for (const registration of registrations) {
        const candidate = readCandidate(registration, pointer, geometry)
        if (!candidate) continue
        if (
          !winner ||
          candidate.distance < winner.distance - WINNER_EPSILON ||
          (activeRegistration === candidate.registration &&
            winner.registration !== activeRegistration &&
            candidate.distance <= winner.distance + WINNER_EPSILON)
        ) {
          winner = candidate
        }
      }
    }

    return winner
  }

  function notifyRegistrations(winner: Candidate | null) {
    activeRegistration = winner?.registration ?? null
    for (const registration of registrations) {
      if (registration === winner?.registration && winner) {
        notify(registration, {
          isActive: true,
          point: winner.point,
        })
      } else if (registration.isActive) {
        notify(registration, { isActive: false, point: ORIGIN })
      }
    }
  }

  function evaluate(pointer: CanvasHandleProximityPoint | null) {
    lastPointer = pointer
    notifyRegistrations(selectWinningCandidate(pointer))
  }

  function register(options: CanvasHandleProximityRegistration) {
    const registration: RegisteredHandle = {
      ...options,
      isActive: false,
      point: ORIGIN,
    }
    registrations.add(registration)

    return () => {
      const wasActive = registration === activeRegistration
      registrations.delete(registration)
      if (wasActive) activeRegistration = null
      evaluate(lastPointer)
    }
  }

  function setSuspended(suspended: boolean) {
    if (isSuspended === suspended) return
    isSuspended = suspended
    evaluate(lastPointer)
  }

  return {
    register,
    movePointer: evaluate,
    refresh: () => evaluate(lastPointer),
    setSuspended,
    clear: () => {
      lastPointer = null
      activeRegistration = null
      registrations.clear()
    },
  }
}

export type CanvasHandleProximityCoordinator = ReturnType<
  typeof createCanvasHandleProximityCoordinator
>
