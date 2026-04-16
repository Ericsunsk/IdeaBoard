import { computed, onUnmounted, ref } from 'vue'
import {
  clampColorValue,
  DEFAULT_COLOR_SWATCHES,
  hexToRgb,
  hslToRgb,
  normalizeHexColor,
  rgbToHex,
  rgbToHsl,
} from '../components/canvas/nodes/colorPopoverUtils'
import { pickHexColor } from '../utils/eyedropper'

interface UseColorPickerStateOptions {
  initialHex?: string
  syncHslOnHexInput?: boolean
  onColorChange?: (hex: string) => void
}

export function useColorPickerState(options: UseColorPickerStateOptions = {}) {
  const quickSwatches = DEFAULT_COLOR_SWATCHES
  const spectrumRef = ref<HTMLElement | HTMLElement[] | null>(null)
  const hex = ref('#A3E635')
  const hue = ref(84)
  const saturation = ref(78)
  const lightness = ref(56)
  let activePointerId: number | null = null

  function notifyColorChange(value: string) {
    options.onColorChange?.(value)
  }

  function syncFromHex(value: string, notifyChange = false) {
    const normalized = normalizeHexColor(value)
    const { r, g, b } = hexToRgb(normalized)
    const hsl = rgbToHsl(r, g, b)
    hex.value = normalized
    hue.value = hsl.h
    saturation.value = hsl.s
    lightness.value = hsl.l
    if (notifyChange) notifyColorChange(normalized)
    return normalized
  }

  function applyCurrentHsl(notifyChange = true) {
    hue.value = clampColorValue(hue.value, 0, 360)
    saturation.value = clampColorValue(saturation.value, 0, 100)
    lightness.value = clampColorValue(lightness.value, 0, 100)
    const rgb = hslToRgb(hue.value, saturation.value, lightness.value)
    const normalized = rgbToHex(rgb.r, rgb.g, rgb.b)
    hex.value = normalized
    if (notifyChange) notifyColorChange(normalized)
    return normalized
  }

  function handleHexInput(rawInput: string) {
    const raw = rawInput.trim().toUpperCase()
    const display = raw ? (raw.startsWith('#') ? raw : `#${raw}`) : '#'
    hex.value = display.slice(0, 7)
    if (!/^#?[0-9A-F]{6}$/.test(raw)) return null
    const normalized = normalizeHexColor(raw)
    if (options.syncHslOnHexInput) {
      return syncFromHex(normalized, true)
    }
    hex.value = normalized
    notifyColorChange(normalized)
    return normalized
  }

  function commitHex(fallbackHex = '#A3E635') {
    const raw = hex.value.trim().toUpperCase()
    if (/^#?[0-9A-F]{6}$/.test(raw)) {
      return syncFromHex(raw, true)
    }
    return syncFromHex(fallbackHex, true)
  }

  function getSpectrumElement() {
    const raw = spectrumRef.value
    if (!raw) return null
    return Array.isArray(raw) ? (raw[0] || null) : raw
  }

  function updateColorFromSpectrumClient(clientX: number, clientY: number) {
    const el = getSpectrumElement()
    if (!el) return null
    const rect = el.getBoundingClientRect()
    if (!rect.width || !rect.height) return null
    const xRatio = clampColorValue((clientX - rect.left) / rect.width, 0, 1)
    const yRatio = clampColorValue((clientY - rect.top) / rect.height, 0, 1)
    saturation.value = Math.round(xRatio * 100)
    lightness.value = Math.round((1 - yRatio) * 100)
    return applyCurrentHsl(true)
  }

  function onSpectrumPointerMove(event: PointerEvent) {
    if (activePointerId !== event.pointerId) return
    updateColorFromSpectrumClient(event.clientX, event.clientY)
  }

  function stopSpectrumDragging() {
    if (activePointerId === null) return
    activePointerId = null
    window.removeEventListener('pointermove', onSpectrumPointerMove)
    window.removeEventListener('pointerup', onSpectrumPointerUp)
    window.removeEventListener('pointercancel', onSpectrumPointerUp)
  }

  function onSpectrumPointerUp(event: PointerEvent) {
    if (activePointerId !== event.pointerId) return
    stopSpectrumDragging()
  }

  function onSpectrumPointerDown(event: PointerEvent) {
    activePointerId = event.pointerId
    const target = event.currentTarget as HTMLElement | null
    target?.setPointerCapture?.(event.pointerId)
    updateColorFromSpectrumClient(event.clientX, event.clientY)
    window.addEventListener('pointermove', onSpectrumPointerMove)
    window.addEventListener('pointerup', onSpectrumPointerUp)
    window.addEventListener('pointercancel', onSpectrumPointerUp)
  }

  function setSpectrumRef(el: HTMLElement | null) {
    spectrumRef.value = el
  }

  function setHue(value: number) {
    hue.value = value
    return applyCurrentHsl(true)
  }

  function setSaturation(value: number) {
    saturation.value = value
    return applyCurrentHsl(true)
  }

  function setLightness(value: number) {
    lightness.value = value
    return applyCurrentHsl(true)
  }

  async function pickEyeDropperColor(fallbackHex: string) {
    return normalizeHexColor((await pickHexColor()) || fallbackHex)
  }

  const spectrumStyle = computed(() => ({
    background:
      `linear-gradient(to top, #000, rgba(0,0,0,0)), linear-gradient(to right, #fff, hsl(${hue.value}, 100%, 50%))`,
  }))

  const cursorStyle = computed(() => ({
    left: `${saturation.value}%`,
    top: `${100 - lightness.value}%`,
  }))

  syncFromHex(options.initialHex ?? '#A3E635', false)

  onUnmounted(() => {
    stopSpectrumDragging()
  })

  return {
    hex,
    hue,
    saturation,
    lightness,
    quickSwatches,
    spectrumStyle,
    cursorStyle,
    syncFromHex,
    handleHexInput,
    commitHex,
    setHue,
    setSaturation,
    setLightness,
    setSpectrumRef,
    onSpectrumPointerDown,
    pickEyeDropperColor,
  }
}
