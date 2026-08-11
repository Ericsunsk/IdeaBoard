export type CanvasHandleProximityDirection = 'input' | 'output'

export type CanvasHandleProximityPoint = {
  x: number
  y: number
}

type CanvasHandleProximityState = {
  isActive: boolean
  isReturning: boolean
  point: CanvasHandleProximityPoint
}

export type CanvasHandleAnchorRect = {
  left: number
  top: number
  width: number
  height: number
}

type CanvasHandleProximityRegistration = {
  direction: CanvasHandleProximityDirection
  getAnchorRect: () => CanvasHandleAnchorRect | null
  isEnabled: () => boolean
  onStateChange: (state: CanvasHandleProximityState) => void
}

type RegisteredHandle = CanvasHandleProximityRegistration & CanvasHandleProximityState & {
  returnTimer: ReturnType<typeof setTimeout> | null
}

type Candidate = {
  registration: RegisteredHandle
  distance: number
  point: CanvasHandleProximityPoint
}

const PROXIMITY_RADIUS = 56
const SECTOR_COSINE = 0.5
const WINNER_EPSILON = 0.5
const RETURN_DURATION_MS = 180
const ORIGIN: CanvasHandleProximityPoint = { x: 0, y: 0 }

function readCandidate(
  registration: RegisteredHandle,
  pointer: CanvasHandleProximityPoint,
): Candidate | null {
  if (!registration.isEnabled()) return null
  const bounds = registration.getAnchorRect()
  if (!bounds) return null

  const point = {
    x: pointer.x - (bounds.left + bounds.width / 2),
    y: pointer.y - (bounds.top + bounds.height / 2),
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
    registration.isReturning === state.isReturning &&
    registration.point.x === state.point.x &&
    registration.point.y === state.point.y
  ) {
    return
  }
  registration.isActive = state.isActive
  registration.isReturning = state.isReturning
  registration.point = state.point
  registration.onStateChange(state)
}

function cancelReturn(registration: RegisteredHandle) {
  if (registration.returnTimer === null) return
  clearTimeout(registration.returnTimer)
  registration.returnTimer = null
}

export function createCanvasHandleProximityCoordinator() {
  const registrations = new Set<RegisteredHandle>()
  let activeRegistration: RegisteredHandle | null = null
  let lastPointer: CanvasHandleProximityPoint | null = null

  function startReturn(registration: RegisteredHandle) {
    if (registration.isReturning) return
    notify(registration, { isActive: false, isReturning: true, point: ORIGIN })
    registration.returnTimer = setTimeout(() => {
      registration.returnTimer = null
      if (registration.isActive || !registrations.has(registration)) return
      notify(registration, { isActive: false, isReturning: false, point: ORIGIN })
    }, RETURN_DURATION_MS)
  }

  function evaluate(pointer: CanvasHandleProximityPoint | null) {
    lastPointer = pointer
    let winner: Candidate | null = null

    if (pointer) {
      for (const registration of registrations) {
        const candidate = readCandidate(registration, pointer)
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

    activeRegistration = winner?.registration ?? null
    for (const registration of registrations) {
      if (registration === winner?.registration && winner) {
        cancelReturn(registration)
        notify(registration, {
          isActive: true,
          isReturning: false,
          point: winner.point,
        })
      } else if (!registration.isEnabled()) {
        cancelReturn(registration)
        notify(registration, { isActive: false, isReturning: false, point: ORIGIN })
      } else if (registration.isActive) {
        startReturn(registration)
      }
    }
  }

  function register(options: CanvasHandleProximityRegistration) {
    const registration: RegisteredHandle = {
      ...options,
      isActive: false,
      isReturning: false,
      point: ORIGIN,
      returnTimer: null,
    }
    registrations.add(registration)

    return () => {
      const wasActive = registration === activeRegistration
      registrations.delete(registration)
      cancelReturn(registration)
      notify(registration, { isActive: false, isReturning: false, point: ORIGIN })
      if (wasActive) activeRegistration = null
      evaluate(lastPointer)
    }
  }

  return {
    register,
    movePointer: evaluate,
    refresh: () => evaluate(lastPointer),
    clear: () => {
      lastPointer = null
      activeRegistration = null
      for (const registration of registrations) {
        cancelReturn(registration)
        notify(registration, { isActive: false, isReturning: false, point: ORIGIN })
      }
      registrations.clear()
    },
  }
}

export type CanvasHandleProximityCoordinator = ReturnType<
  typeof createCanvasHandleProximityCoordinator
>
