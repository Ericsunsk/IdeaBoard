import { useEventListener } from '@vueuse/core'
import { computed, toValue, type MaybeRefOrGetter } from 'vue'

type DismissableLayerOptions = {
  enabled: MaybeRefOrGetter<boolean>
  ignore?: ReadonlyArray<string>
  onDismiss: (event: KeyboardEvent | PointerEvent) => void
  ownerDocument?: () => Document | null
}

export function useDismissableLayer(options: DismissableLayerOptions) {
  function resolveDocument() {
    if (options.ownerDocument) return options.ownerDocument()
    if (typeof document === 'undefined') return null
    return document
  }

  const listenerTarget = computed(() => (toValue(options.enabled) ? resolveDocument() : null))

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

  useEventListener(listenerTarget, 'pointerdown', handlePointerDown, true)
  useEventListener(listenerTarget, 'keydown', handleKeyDown, true)
}
