import { resolveApiUrl } from '../config/runtimeUrls'

export type UserEventName = 'task.updated' | 'payment.updated' | 'media.derivative.updated'

type UserEventListener = (message: MessageEvent<string>) => void

const listeners = new Map<UserEventName, Set<UserEventListener>>()
const transportListeners = new Map<UserEventName, EventListener>()
let source: EventSource | null = null
let errorListeners = new Set<() => void>()

function ensureSource(): EventSource | null {
  if (typeof EventSource === 'undefined') return null
  if (source) return source
  source = new EventSource(resolveApiUrl('/user/events'), { withCredentials: true })
  source.onerror = () => {
    for (const listener of errorListeners) listener()
  }
  for (const eventName of listeners.keys()) installTransportListener(eventName)
  return source
}

function installTransportListener(eventName: UserEventName): void {
  if (!source || transportListeners.has(eventName)) return
  const transportListener: EventListener = (event) => {
    for (const listener of listeners.get(eventName) ?? []) {
      listener(event as MessageEvent<string>)
    }
  }
  transportListeners.set(eventName, transportListener)
  source.addEventListener(eventName, transportListener)
}

function closeWhenIdle(): void {
  if ([...listeners.values()].some((entries) => entries.size > 0)) return
  if (source) {
    for (const [eventName, listener] of transportListeners) {
      source.removeEventListener(eventName, listener)
    }
    source.close()
  }
  source = null
  transportListeners.clear()
  errorListeners = new Set()
}

export function subscribeUserEvent(
  eventName: UserEventName,
  listener: UserEventListener,
  onError?: () => void,
): () => void {
  let entries = listeners.get(eventName)
  if (!entries) {
    entries = new Set()
    listeners.set(eventName, entries)
  }
  entries.add(listener)
  if (onError) errorListeners.add(onError)
  ensureSource()
  installTransportListener(eventName)

  return () => {
    entries?.delete(listener)
    if (onError) errorListeners.delete(onError)
    closeWhenIdle()
  }
}
