import { toValue, watchEffect, type MaybeRefOrGetter } from 'vue'
import {
  useOverlayInteractionLifecycle,
  type OverlayDismissal,
} from '../../../shared/composables/useOverlayInteractionLifecycle'

type CanvasCustomLayerRoot = MaybeRefOrGetter<HTMLElement | null | undefined>

type CanvasCustomLayerDismissEvent = KeyboardEvent | PointerEvent

export type UseCanvasCustomLayerOverlayOptions = {
  visible: MaybeRefOrGetter<boolean>
  roots?: readonly CanvasCustomLayerRoot[]
  ignore?: readonly string[] | undefined
  ignoreEscape?: readonly string[]
  dismissibleBy?: readonly Extract<OverlayDismissal, 'escape' | 'outside-pointer'>[]
  ownerDocument?: () => Document | null
  shouldDismiss?:
    ((dismissal: OverlayDismissal, event: CanvasCustomLayerDismissEvent) => boolean) | undefined
  onDismiss: (dismissal: OverlayDismissal, event: CanvasCustomLayerDismissEvent) => void
}

type CanvasCustomLayerEntry = {
  isVisible: () => boolean
  openSequence: () => number
  containsPointerEvent: (event: PointerEvent) => boolean
  ignoresEscapeEvent: (event: KeyboardEvent) => boolean
  requestClose: (dismissal: OverlayDismissal, event: CanvasCustomLayerDismissEvent) => boolean
}

type CanvasCustomLayerStack = {
  register: (entry: CanvasCustomLayerEntry) => () => void
}

const canvasCustomLayerStacks = new WeakMap<Document, CanvasCustomLayerStack>()
let nextCanvasCustomLayerOpenSequence = 0

export function useCanvasCustomLayerOverlay(options: UseCanvasCustomLayerOverlayOptions) {
  let wasVisible = false
  let openSequence = 0
  let dismissalContext:
    { dismissal: OverlayDismissal; event: CanvasCustomLayerDismissEvent } | undefined
  const lifecycle = useOverlayInteractionLifecycle({
    visible: options.visible,
    surface: {
      dismissibleBy: options.dismissibleBy ?? ['outside-pointer', 'escape'],
      restoreFocus: false,
    },
    onVisibilityChange: (visible) => {
      if (visible || !dismissalContext) return
      options.onDismiss(dismissalContext.dismissal, dismissalContext.event)
    },
  })
  const entry: CanvasCustomLayerEntry = {
    isVisible: () => Boolean(toValue(options.visible)),
    openSequence: () => openSequence,
    containsPointerEvent: (event) =>
      eventMatchesRoots(event, options.roots) || eventMatchesSelectors(event, options.ignore),
    ignoresEscapeEvent: (event) => eventMatchesSelectors(event, options.ignoreEscape),
    requestClose: (dismissal, event) => {
      if (options.shouldDismiss && !options.shouldDismiss(dismissal, event)) return false
      dismissalContext = { dismissal, event }
      try {
        return lifecycle.requestClose(dismissal)
      } finally {
        dismissalContext = undefined
      }
    },
  }

  watchEffect((onCleanup) => {
    const isVisible = Boolean(toValue(options.visible))
    if (!isVisible) {
      wasVisible = false
      return
    }
    if (!wasVisible) {
      wasVisible = true
      openSequence = ++nextCanvasCustomLayerOpenSequence
    }
    const ownerDocument = resolveOwnerDocument(options)
    if (!ownerDocument) return

    const unregister = getCanvasCustomLayerStack(ownerDocument).register(entry)
    onCleanup(unregister)
  })

  return {
    canDismiss: lifecycle.canDismiss,
    requestClose: entry.requestClose,
  }
}

function resolveOwnerDocument(options: UseCanvasCustomLayerOverlayOptions) {
  const explicitDocument = options.ownerDocument?.()
  if (explicitDocument) return explicitDocument

  for (const root of options.roots ?? []) {
    const element = toValue(root)
    if (element?.ownerDocument) return element.ownerDocument
  }
  return typeof document === 'undefined' ? null : document
}

function getCanvasCustomLayerStack(ownerDocument: Document) {
  const existing = canvasCustomLayerStacks.get(ownerDocument)
  if (existing) return existing

  const stack = createCanvasCustomLayerStack(ownerDocument)
  canvasCustomLayerStacks.set(ownerDocument, stack)
  return stack
}

function createCanvasCustomLayerStack(ownerDocument: Document): CanvasCustomLayerStack {
  const activeLayers: CanvasCustomLayerEntry[] = []
  let listening = false

  function topmostLayer() {
    let topmost: CanvasCustomLayerEntry | undefined
    for (const layer of activeLayers) {
      if (layer.isVisible() && (!topmost || layer.openSequence() > topmost.openSequence())) {
        topmost = layer
      }
    }
    return topmost
  }

  function handlePointerDown(event: PointerEvent) {
    const layer = topmostLayer()
    if (!layer || layer.containsPointerEvent(event)) return
    layer.requestClose('outside-pointer', event)
  }

  function handleKeyDown(event: KeyboardEvent) {
    if (event.key !== 'Escape') return
    const layer = topmostLayer()
    if (!layer || layer.ignoresEscapeEvent(event)) return
    if (!layer.requestClose('escape', event)) return

    event.preventDefault()
    event.stopImmediatePropagation()
  }

  function startListening() {
    if (listening) return
    listening = true
    ownerDocument.addEventListener('pointerdown', handlePointerDown, true)
    ownerDocument.addEventListener('keydown', handleKeyDown, true)
  }

  function stopListening() {
    if (!listening) return
    listening = false
    ownerDocument.removeEventListener('pointerdown', handlePointerDown, true)
    ownerDocument.removeEventListener('keydown', handleKeyDown, true)
  }

  return {
    register(entry) {
      activeLayers.push(entry)
      startListening()
      let registered = true

      return () => {
        if (!registered) return
        registered = false
        const index = activeLayers.lastIndexOf(entry)
        if (index >= 0) activeLayers.splice(index, 1)
        if (activeLayers.length === 0) stopListening()
      }
    },
  }
}

function eventMatchesRoots(event: Event, roots: readonly CanvasCustomLayerRoot[] | undefined) {
  if (!roots?.length) return false
  const path = event.composedPath()
  return roots.some((root) => {
    const element = toValue(root)
    return Boolean(
      element &&
      (path.includes(element) || (event.target instanceof Node && element.contains(event.target))),
    )
  })
}

function eventMatchesSelectors(event: Event, selectors: readonly string[] | undefined) {
  if (!selectors?.length) return false
  return event
    .composedPath()
    .some(
      (target) =>
        target instanceof Element &&
        selectors.some((selector) => target.matches(selector) || Boolean(target.closest(selector))),
    )
}
