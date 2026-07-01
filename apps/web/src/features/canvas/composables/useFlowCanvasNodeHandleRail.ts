import type { Ref } from 'vue'
import { tryOnScopeDispose, useTimeoutFn } from '@vueuse/core'
import { computed, ref } from 'vue'

/**
 * Tracks transient handle visibility for handles rendered inside the node frame.
 * Geometry is owned by Vue Flow <Handle> placement and CSS.
 */
export function useFlowCanvasNodeHandleRail(flowNodeRef: Ref<HTMLElement | null>) {
  const isPointerOverNode = ref(false)
  const isKeyboardFocusWithinNode = ref(false)
  const isTouchHandleHintActive = ref(false)

  const touchHintTimer = useTimeoutFn(
    () => {
      isTouchHandleHintActive.value = false
    },
    2400,
    { immediate: false },
  )

  const showHandleControls = computed(
    () =>
      isPointerOverNode.value || isKeyboardFocusWithinNode.value || isTouchHandleHintActive.value,
  )

  function syncKeyboardFocus() {
    const root = flowNodeRef.value
    isKeyboardFocusWithinNode.value = !!root?.querySelector(':focus-visible')
  }

  function clearTouchHint() {
    isTouchHandleHintActive.value = false
    touchHintTimer.stop()
  }

  function refreshTransientHandleVisibility() {
    clearTouchHint()

    const activeEl = document.activeElement
    if (
      activeEl instanceof HTMLElement &&
      activeEl.classList.contains('flow-canvas-node__handle-aggregate')
    ) {
      activeEl.blur()
    }

    const root = flowNodeRef.value
    if (!root) return
    isPointerOverNode.value = root.matches(':hover')
    syncKeyboardFocus()
  }

  function onNodeMouseEnter() {
    isPointerOverNode.value = true
  }

  function onNodeMouseLeave() {
    isPointerOverNode.value = false
  }

  function onNodeFocusIn() {
    syncKeyboardFocus()
  }

  function onNodeFocusOut(event: FocusEvent) {
    const nextTarget = event.relatedTarget
    if (nextTarget instanceof Node && flowNodeRef.value?.contains(nextTarget)) {
      syncKeyboardFocus()
      return
    }
    isKeyboardFocusWithinNode.value = false
  }

  function onNodePointerDown(event: PointerEvent) {
    if (event.pointerType === 'mouse') return
    isTouchHandleHintActive.value = true
    touchHintTimer.start()
  }

  tryOnScopeDispose(clearTouchHint)

  return {
    showHandleControls,
    refreshTransientHandleVisibility,
    onNodeMouseEnter,
    onNodeMouseLeave,
    onNodeFocusIn,
    onNodeFocusOut,
    onNodePointerDown,
  }
}
