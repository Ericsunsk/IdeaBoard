import { onScopeDispose, toValue, watch, type MaybeRefOrGetter } from 'vue'

type DismissableLayerOptions = {
  enabled: MaybeRefOrGetter<boolean>
  ignore?: ReadonlyArray<string>
  onDismiss: (event: KeyboardEvent | PointerEvent) => void
  ownerDocument?: () => Document | null
}

export function useDismissableLayer(options: DismissableLayerOptions) {
  let removeListeners: (() => void) | null = null

  function resolveDocument() {
    if (options.ownerDocument) return options.ownerDocument()
    if (typeof document === 'undefined') return null
    return document
  }

  function isInsideIgnoredLayer(event: Event) {
    const selectors = options.ignore ?? []
    if (selectors.length === 0) return false

    return event.composedPath().some((target) => {
      if (!(target instanceof Element)) return false
      return selectors.some((selector) => target.matches(selector) || !!target.closest(selector))
    })
  }

  function handlePointerDown(event: PointerEvent) {
    if (!toValue(options.enabled) || isInsideIgnoredLayer(event)) return
    options.onDismiss(event)
  }

  function handleKeyDown(event: KeyboardEvent) {
    if (!toValue(options.enabled) || event.key !== 'Escape') return
    options.onDismiss(event)
  }

  function addListeners() {
    if (removeListeners) return
    const doc = resolveDocument()
    if (!doc) return

    doc.addEventListener('pointerdown', handlePointerDown, true)
    doc.addEventListener('keydown', handleKeyDown, true)
    removeListeners = () => {
      doc.removeEventListener('pointerdown', handlePointerDown, true)
      doc.removeEventListener('keydown', handleKeyDown, true)
      removeListeners = null
    }
  }

  function cleanupListeners() {
    removeListeners?.()
  }

  const stopWatching = watch(
    () => toValue(options.enabled),
    (enabled) => {
      if (enabled) addListeners()
      else cleanupListeners()
    },
    { immediate: true },
  )

  onScopeDispose(() => {
    stopWatching()
    cleanupListeners()
  })
}
