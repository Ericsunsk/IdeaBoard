import type { Ref } from 'vue'
import { tryOnScopeDispose, useTimeoutFn } from '@vueuse/core'
import { computed, ref, watch } from 'vue'

/**
 * Tracks transient handle visibility for handles rendered inside the node frame.
 * Geometry is owned by Vue Flow <Handle> placement and CSS.
 */
export function useFlowCanvasNodeHandleRail(flowNodeRef: Ref<HTMLElement | null>) {
  const isPointerOverNode = ref(false)
  const isKeyboardFocusWithinNode = ref(false)
  const isTransientHandleHintActive = ref(false)

  const transientHintTimer = useTimeoutFn(
    () => {
      isTransientHandleHintActive.value = false
    },
    2400,
    { immediate: false },
  )

  const shouldShowHandleControls = computed(
    () =>
      isPointerOverNode.value ||
      isKeyboardFocusWithinNode.value ||
      isTransientHandleHintActive.value,
  )

  function syncKeyboardFocus() {
    const root = flowNodeRef.value
    isKeyboardFocusWithinNode.value = !!root?.querySelector(':focus-visible')
  }

  function clearTransientHint() {
    isTransientHandleHintActive.value = false
    transientHintTimer.stop()
  }

  function refreshTransientHandleVisibility() {
    clearTransientHint()

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
    isTransientHandleHintActive.value = true
    transientHintTimer.start()
  }

  watch(
    flowNodeRef,
    (root, _, onCleanup) => {
      isPointerOverNode.value = false
      isKeyboardFocusWithinNode.value = false
      if (!root) return

      root.addEventListener('mouseenter', onNodeMouseEnter)
      root.addEventListener('mouseleave', onNodeMouseLeave)
      root.addEventListener('focusin', onNodeFocusIn)
      root.addEventListener('focusout', onNodeFocusOut)
      root.addEventListener('pointerdown', onNodePointerDown, true)

      onCleanup(() => {
        root.removeEventListener('mouseenter', onNodeMouseEnter)
        root.removeEventListener('mouseleave', onNodeMouseLeave)
        root.removeEventListener('focusin', onNodeFocusIn)
        root.removeEventListener('focusout', onNodeFocusOut)
        root.removeEventListener('pointerdown', onNodePointerDown, true)
      })
    },
    { immediate: true, flush: 'sync' },
  )

  // Intentional `tryOnScopeDispose`: the rail state resets safely even when
  // the composable is instantiated outside a component scope.
  tryOnScopeDispose(() => {
    isPointerOverNode.value = false
    isKeyboardFocusWithinNode.value = false
    clearTransientHint()
  })

  return {
    shouldShowHandleControls,
    refreshTransientHandleVisibility,
  }
}
