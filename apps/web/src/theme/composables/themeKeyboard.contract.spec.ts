/// <reference types="node" />

import { ref } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { resolveInteractiveControlState } from '../../core/interaction/interactiveControlState'
import { createThemeKeyboardHandler, THEME_SEGMENT_MODES } from './themeKeyboard'
import type { StoredThemeMode, ThemeMode } from './themeMode'

function createSegmentGroup() {
  const group = document.createElement('div')
  group.setAttribute('role', 'radiogroup')
  group.setAttribute('aria-label', 'Theme')
  const radios = THEME_SEGMENT_MODES.map((mode) => {
    const radio = document.createElement('button')
    radio.setAttribute('role', 'radio')
    radio.textContent = mode
    group.appendChild(radio)
    return radio
  })
  document.body.appendChild(group)
  return {
    group,
    radios,
    radioAt: (index: number): HTMLElement => {
      const radio = radios[index]
      if (!radio) throw new Error(`Theme segment radio ${index} is missing`)
      return radio
    },
  }
}

function setupKeyboard(initial: StoredThemeMode) {
  const mode = ref<StoredThemeMode>(initial)
  const setTheme = vi.fn((_next: ThemeMode) => undefined)
  const handler = createThemeKeyboardHandler({ mode, setTheme })
  return { mode, setTheme, handler }
}

function pressKey(group: HTMLElement, key: string): KeyboardEvent {
  const event = new KeyboardEvent('keydown', { key })
  group.dispatchEvent(event)
  return event
}

function pressKeyWithDefaultSpy(
  group: HTMLElement,
  key: string,
): {
  event: KeyboardEvent
  preventDefault: ReturnType<typeof vi.spyOn>
} {
  const event = new KeyboardEvent('keydown', { key })
  const preventDefault = vi.spyOn(event, 'preventDefault')
  group.dispatchEvent(event)
  return { event, preventDefault }
}

describe('theme keyboard control state and focus (theme/control-state-and-focus)', () => {
  beforeEach(() => {
    vi.stubGlobal('requestAnimationFrame', (callback: FrameRequestCallback) => {
      callback(0)
      return 0
    })
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    document.body.innerHTML = ''
  })

  it('moves forward through light, dark, and auto with ArrowRight and wraps', () => {
    const { group, radios } = createSegmentGroup()
    const { mode, setTheme, handler } = setupKeyboard('light')
    group.addEventListener('keydown', handler)

    const { preventDefault } = pressKeyWithDefaultSpy(group, 'ArrowRight')
    expect(preventDefault).toHaveBeenCalledOnce()
    expect(setTheme).toHaveBeenLastCalledWith('dark')
    expect(document.activeElement).toBe(radios[1])

    mode.value = 'dark'
    pressKey(group, 'ArrowRight')
    expect(setTheme).toHaveBeenLastCalledWith('auto')
    expect(document.activeElement).toBe(radios[2])

    mode.value = 'auto'
    pressKey(group, 'ArrowRight')
    expect(setTheme).toHaveBeenLastCalledWith('light')
    expect(document.activeElement).toBe(radios[0])
    expect(setTheme).toHaveBeenCalledTimes(3)
  })

  it('moves backward with ArrowLeft and wraps from light to auto', () => {
    const { group, radios } = createSegmentGroup()
    const { mode, setTheme, handler } = setupKeyboard('dark')
    group.addEventListener('keydown', handler)

    pressKey(group, 'ArrowLeft')
    expect(setTheme).toHaveBeenLastCalledWith('light')
    expect(document.activeElement).toBe(radios[0])

    mode.value = 'light'
    pressKey(group, 'ArrowLeft')
    expect(setTheme).toHaveBeenLastCalledWith('auto')
    expect(document.activeElement).toBe(radios[2])
  })

  it('jumps to the first and last option with Home and End', () => {
    const { group, radios } = createSegmentGroup()
    const { setTheme, handler } = setupKeyboard('dark')
    group.addEventListener('keydown', handler)

    pressKey(group, 'Home')
    expect(setTheme).toHaveBeenLastCalledWith('light')
    expect(document.activeElement).toBe(radios[0])

    pressKey(group, 'End')
    expect(setTheme).toHaveBeenLastCalledWith('auto')
    expect(document.activeElement).toBe(radios[2])
  })

  it('ignores unhandled keys without preventing default or switching theme', () => {
    const { group } = createSegmentGroup()
    const { setTheme, handler } = setupKeyboard('light')
    group.addEventListener('keydown', handler)

    const { preventDefault } = pressKeyWithDefaultSpy(group, 'Enter')
    expect(preventDefault).not.toHaveBeenCalled()
    expect(setTheme).not.toHaveBeenCalled()
    expect(document.activeElement).toBe(document.body)
  })

  it('moves visible focus to the newly selected option', () => {
    const { group, radios, radioAt } = createSegmentGroup()
    const { handler } = setupKeyboard('light')
    group.addEventListener('keydown', handler)

    radioAt(0).focus()
    expect(document.activeElement).toBe(radios[0])

    pressKey(group, 'ArrowRight')
    expect(document.activeElement).toBe(radios[1])
  })

  it('keeps loading and disabled controls keyboard-focusable with stable state', () => {
    const { group, radios, radioAt } = createSegmentGroup()
    const { setTheme, handler } = setupKeyboard('light')
    group.addEventListener('keydown', handler)

    const idle = resolveInteractiveControlState()
    radioAt(0).setAttribute('data-control-state', idle['data-control-state'])
    radioAt(1).setAttribute('data-control-state', 'loading')
    radioAt(1).setAttribute('aria-busy', 'true')
    radioAt(2).setAttribute('data-control-state', 'disabled')

    pressKey(group, 'ArrowRight')
    expect(setTheme).toHaveBeenLastCalledWith('dark')
    expect(document.activeElement).toBe(radios[1])
    expect(radios[1]).toHaveAttribute('data-control-state', 'loading')
    expect(radios[1]).toHaveAttribute('aria-busy', 'true')

    pressKey(group, 'End')
    expect(setTheme).toHaveBeenLastCalledWith('auto')
    expect(document.activeElement).toBe(radios[2])
    expect(radios[2]).toHaveAttribute('data-control-state', 'disabled')
  })
})
