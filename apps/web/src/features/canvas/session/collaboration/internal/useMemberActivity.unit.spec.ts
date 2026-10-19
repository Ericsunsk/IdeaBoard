import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'
import { effectScope, ref } from 'vue'
import { useMemberActivity } from './useMemberActivity'
import type { PresenceActivity } from '../presence/state'

const scopes: ReturnType<typeof effectScope>[] = []
function setup() {
  const scope = effectScope()
  scopes.push(scope)
  const isEnabled = ref(true)
  const state = ref<PresenceActivity | null>(null)
  const activity = scope.run(() => useMemberActivity({ isEnabled, state }))!
  return { scope, isEnabled, state, activity }
}
beforeEach(() => vi.useFakeTimers())
afterEach(() => {
  scopes.splice(0).forEach((scope) => scope.stop())
  vi.restoreAllMocks()
  vi.useRealTimers()
})

describe('member editing activity', () => {
  test('expires 800ms after the last edit and has no idle renewals', () => {
    const { activity, state } = setup()
    activity.recordEdit()
    vi.advanceTimersByTime(700)
    activity.recordEdit()
    vi.advanceTimersByTime(799)
    expect(activity.isEditing.value).toBe(true)
    vi.advanceTimersByTime(1)
    expect(state.value).toBeNull()
    expect(vi.getTimerCount()).toBe(0)
  })

  test('holds overlapping gestures through idle expiry, renews, and ends immediately', () => {
    const { activity, state } = setup()
    activity.recordEdit()
    activity.begin('drag')
    activity.begin('resize')
    const first = state.value!.sequence
    vi.advanceTimersByTime(6_000)
    expect(state.value!.sequence).toBeGreaterThan(first)
    activity.end('drag')
    activity.recordEdit()
    expect(activity.isEditing.value).toBe(true)
    activity.end('resize')
    expect(activity.isEditing.value).toBe(false)
    expect(vi.getTimerCount()).toBe(0)
  })

  test.each(['pointercancel', 'lostpointercapture', 'pointerup', 'mouseup', 'touchend'])(
    '%s clears a missing gesture end',
    (eventName) => {
      const { activity } = setup()
      activity.begin('drag')
      window.dispatchEvent(new Event(eventName))
      vi.advanceTimersByTime(0)
      expect(activity.isEditing.value).toBe(false)
      expect(vi.getTimerCount()).toBe(0)
    },
  )

  test('clears on blur and ignores background edits without resuming old gestures', () => {
    const { activity } = setup()
    activity.begin('drag')
    window.dispatchEvent(new Event('blur'))
    activity.recordEdit()
    expect(activity.isEditing.value).toBe(false)
    window.dispatchEvent(new Event('focus'))
    vi.advanceTimersByTime(10_000)
    expect(activity.isEditing.value).toBe(false)
    activity.recordEdit()
    expect(activity.isEditing.value).toBe(true)
  })

  test('clears hidden state and blocks edits until visible', () => {
    const { activity } = setup()
    activity.recordEdit()
    vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('hidden')
    document.dispatchEvent(new Event('visibilitychange'))
    activity.recordEdit()
    expect(activity.isEditing.value).toBe(false)
    vi.restoreAllMocks()
    document.dispatchEvent(new Event('visibilitychange'))
    expect(activity.isEditing.value).toBe(false)
  })

  test('readiness loss and disposal clear timers and prevent stale activation', () => {
    const { activity, isEnabled, scope } = setup()
    activity.begin('drag')
    isEnabled.value = false
    expect(activity.isEditing.value).toBe(false)
    activity.recordEdit()
    expect(activity.isEditing.value).toBe(false)
    isEnabled.value = true
    activity.recordEdit()
    scope.stop()
    vi.advanceTimersByTime(10_000)
    activity.begin('drag')
    expect(activity.isEditing.value).toBe(false)
    expect(vi.getTimerCount()).toBe(0)
  })
})
