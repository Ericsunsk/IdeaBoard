import { computed, onScopeDispose, ref, toValue, watch, type MaybeRefOrGetter, type Ref } from 'vue'
import type { PresenceActivity } from '../presence/state'

// Editing idle time is behavior, independent of the visual motion duration.
const EDIT_IDLE_MS = 800
const ACTIVITY_RENEW_MS = 2_000

/** One owner for overlapping gestures, accepted local edits and their lease. */
export function useMemberActivity(options: {
  isEnabled: MaybeRefOrGetter<boolean>
  state: Ref<PresenceActivity | null>
}) {
  const gestures = new Set<string>()
  const isEditing = computed(() => options.state.value !== null)
  const isForeground = ref(typeof document === 'undefined' || document.visibilityState !== 'hidden')
  let sequence = 0
  let idleTimer: ReturnType<typeof setTimeout> | undefined
  let renewTimer: ReturnType<typeof setTimeout> | undefined
  let pointerEndTimer: ReturnType<typeof setTimeout> | undefined
  let isDisposed = false

  function allowed() {
    return !isDisposed && toValue(options.isEnabled) && isForeground.value
  }

  function renew() {
    if (!allowed()) return clear()
    options.state.value = { sequence: ++sequence }
    clearTimeout(renewTimer)
    renewTimer = setTimeout(renew, ACTIVITY_RENEW_MS)
  }

  function settle() {
    if (gestures.size > 0 || idleTimer !== undefined) {
      if (!isEditing.value) renew()
      return
    }
    clearTimeout(renewTimer)
    renewTimer = undefined
    options.state.value = null
  }

  function clear() {
    gestures.clear()
    clearTimeout(pointerEndTimer)
    clearTimeout(idleTimer)
    clearTimeout(renewTimer)
    idleTimer = undefined
    renewTimer = undefined
    options.state.value = null
  }

  function begin(key: string) {
    if (!allowed()) return
    clearTimeout(pointerEndTimer)
    gestures.add(key)
    settle()
  }

  function end(key: string) {
    gestures.delete(key)
    settle()
  }

  function recordEdit() {
    if (!allowed() || gestures.size > 0) return
    clearTimeout(idleTimer)
    idleTimer = setTimeout(() => {
      idleTimer = undefined
      settle()
    }, EDIT_IDLE_MS)
    settle()
  }

  function cancelGestures() {
    gestures.clear()
    settle()
  }

  // Let native drag-stop/commit handlers finish before the missing-stop safety net.
  function onPointerEnd() {
    if (gestures.size === 0) return
    clearTimeout(pointerEndTimer)
    pointerEndTimer = setTimeout(cancelGestures, 0)
  }

  function onBlur() {
    isForeground.value = false
    clear()
  }
  function onFocus() {
    isForeground.value = typeof document === 'undefined' || document.visibilityState !== 'hidden'
  }
  function onVisibilityChange() {
    if (document.visibilityState === 'hidden') onBlur()
    else onFocus()
  }
  watch(
    () => toValue(options.isEnabled),
    (isEnabled) => {
      if (!isEnabled) clear()
    },
    { flush: 'sync' },
  )
  if (typeof window !== 'undefined') {
    window.addEventListener('blur', onBlur)
    window.addEventListener('focus', onFocus)
    window.addEventListener('pointerup', onPointerEnd, true)
    window.addEventListener('mouseup', onPointerEnd, true)
    window.addEventListener('touchend', onPointerEnd, true)
    window.addEventListener('pointercancel', cancelGestures, true)
    window.addEventListener('lostpointercapture', cancelGestures, true)
    document.addEventListener('visibilitychange', onVisibilityChange)
  }
  onScopeDispose(() => {
    isDisposed = true
    clear()
    if (typeof window === 'undefined') return
    window.removeEventListener('blur', onBlur)
    window.removeEventListener('focus', onFocus)
    window.removeEventListener('pointerup', onPointerEnd, true)
    window.removeEventListener('mouseup', onPointerEnd, true)
    window.removeEventListener('touchend', onPointerEnd, true)
    window.removeEventListener('pointercancel', cancelGestures, true)
    window.removeEventListener('lostpointercapture', cancelGestures, true)
    document.removeEventListener('visibilitychange', onVisibilityChange)
  })
  return { isEditing, begin, end, recordEdit, clear }
}
