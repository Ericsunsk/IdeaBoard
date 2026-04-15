import { computed, onUnmounted, ref, type Ref } from 'vue'
import type { ColorNodeItem } from '../types/canvasNodes'

type EyeDropperResult = { sRGBHex: string }
type EyeDropperInstance = { open: () => Promise<EyeDropperResult> }
type EyeDropperConstructor = new () => EyeDropperInstance

interface ColorNodeMessageApi {
  warning(content: string): unknown
  error(content: string): unknown
}

interface UseColorNodeOptions {
  colorNodes: Ref<ColorNodeItem[]>
  message: ColorNodeMessageApi
}

function clampColorValue(value: number, min: number, max: number): number {
  return Math.min(max, Math.max(min, value))
}

function hexToRgb(hex: string): { r: number; g: number; b: number } {
  const normalized = hex.replace('#', '')
  return {
    r: parseInt(normalized.slice(0, 2), 16),
    g: parseInt(normalized.slice(2, 4), 16),
    b: parseInt(normalized.slice(4, 6), 16),
  }
}

function rgbToHex(r: number, g: number, b: number): string {
  const toHex = (num: number) => clampColorValue(Math.round(num), 0, 255).toString(16).padStart(2, '0')
  return `#${toHex(r)}${toHex(g)}${toHex(b)}`.toUpperCase()
}

function rgbToHsl(r: number, g: number, b: number): { h: number; s: number; l: number } {
  const rn = r / 255
  const gn = g / 255
  const bn = b / 255
  const max = Math.max(rn, gn, bn)
  const min = Math.min(rn, gn, bn)
  const delta = max - min
  let h = 0
  const l = (max + min) / 2
  let s = 0

  if (delta !== 0) {
    s = delta / (1 - Math.abs(2 * l - 1))
    switch (max) {
      case rn:
        h = ((gn - bn) / delta + (gn < bn ? 6 : 0)) * 60
        break
      case gn:
        h = ((bn - rn) / delta + 2) * 60
        break
      default:
        h = ((rn - gn) / delta + 4) * 60
        break
    }
  }

  return {
    h: Math.round(h),
    s: Math.round(s * 100),
    l: Math.round(l * 100),
  }
}

function hslToRgb(h: number, s: number, l: number): { r: number; g: number; b: number } {
  const hn = ((h % 360) + 360) % 360 / 360
  const sn = clampColorValue(s, 0, 100) / 100
  const ln = clampColorValue(l, 0, 100) / 100

  if (sn === 0) {
    const gray = Math.round(ln * 255)
    return { r: gray, g: gray, b: gray }
  }

  const q = ln < 0.5 ? ln * (1 + sn) : ln + sn - ln * sn
  const p = 2 * ln - q
  const hueToRgb = (t: number) => {
    let tt = t
    if (tt < 0) tt += 1
    if (tt > 1) tt -= 1
    if (tt < 1 / 6) return p + (q - p) * 6 * tt
    if (tt < 1 / 2) return q
    if (tt < 2 / 3) return p + (q - p) * (2 / 3 - tt) * 6
    return p
  }

  return {
    r: Math.round(hueToRgb(hn + 1 / 3) * 255),
    g: Math.round(hueToRgb(hn) * 255),
    b: Math.round(hueToRgb(hn - 1 / 3) * 255),
  }
}

export function normalizeHexColor(input: string): string {
  const raw = input.trim().toUpperCase()
  if (/^#[0-9A-F]{6}$/.test(raw)) return raw
  if (/^[0-9A-F]{6}$/.test(raw)) return `#${raw}`
  return '#A3E635'
}

function getEyeDropperConstructor(): EyeDropperConstructor | null {
  const constructorCandidate = (window as unknown as { EyeDropper?: EyeDropperConstructor }).EyeDropper
  return typeof constructorCandidate === 'function' ? constructorCandidate : null
}

export function useColorNode(options: UseColorNodeOptions) {
  const activeColorPopoverNodeId = ref<number | null>(null)
  const colorPopoverHex = ref('#A3E635')
  const colorPopoverHue = ref(84)
  const colorPopoverSaturation = ref(78)
  const colorPopoverLightness = ref(56)
  const colorSpectrumRef = ref<HTMLElement | HTMLElement[] | null>(null)
  const colorSpectrumStyle = computed(() => ({
    background:
      `linear-gradient(to top, #000, rgba(0,0,0,0)), linear-gradient(to right, #fff, hsl(${colorPopoverHue.value}, 100%, 50%))`,
  }))
  const colorSpectrumCursorStyle = computed(() => ({
    left: `${colorPopoverSaturation.value}%`,
    top: `${100 - colorPopoverLightness.value}%`,
  }))

  let colorSpectrumActivePointerId: number | null = null

  function applyColorPopoverToNode() {
    const nodeId = activeColorPopoverNodeId.value
    if (nodeId === null) return
    const node = options.colorNodes.value.find(item => item.id === nodeId)
    if (!node) return
    colorPopoverHue.value = clampColorValue(colorPopoverHue.value, 0, 360)
    colorPopoverSaturation.value = clampColorValue(colorPopoverSaturation.value, 0, 100)
    colorPopoverLightness.value = clampColorValue(colorPopoverLightness.value, 0, 100)
    const rgb = hslToRgb(colorPopoverHue.value, colorPopoverSaturation.value, colorPopoverLightness.value)
    const hex = rgbToHex(rgb.r, rgb.g, rgb.b)
    colorPopoverHex.value = hex
    node.targetColor = hex
  }

  function setColorPopoverHex(hex: string) {
    const normalized = normalizeHexColor(hex)
    const { r, g, b } = hexToRgb(normalized)
    const hsl = rgbToHsl(r, g, b)
    colorPopoverHex.value = normalized
    colorPopoverHue.value = hsl.h
    colorPopoverSaturation.value = hsl.s
    colorPopoverLightness.value = hsl.l
    applyColorPopoverToNode()
  }

  function getColorSpectrumEl(): HTMLElement | null {
    const raw = colorSpectrumRef.value
    if (!raw) return null
    return Array.isArray(raw) ? (raw[0] || null) : raw
  }

  function updateColorFromSpectrumClient(clientX: number, clientY: number) {
    const el = getColorSpectrumEl()
    if (!el) return
    const rect = el.getBoundingClientRect()
    if (!rect.width || !rect.height) return
    const xRatio = clampColorValue((clientX - rect.left) / rect.width, 0, 1)
    const yRatio = clampColorValue((clientY - rect.top) / rect.height, 0, 1)
    colorPopoverSaturation.value = Math.round(xRatio * 100)
    colorPopoverLightness.value = Math.round((1 - yRatio) * 100)
    applyColorPopoverToNode()
  }

  function onColorSpectrumPointerMove(event: PointerEvent) {
    if (colorSpectrumActivePointerId !== event.pointerId) return
    updateColorFromSpectrumClient(event.clientX, event.clientY)
  }

  function stopColorSpectrumDragging() {
    if (colorSpectrumActivePointerId === null) return
    colorSpectrumActivePointerId = null
    window.removeEventListener('pointermove', onColorSpectrumPointerMove)
    window.removeEventListener('pointerup', onColorSpectrumPointerUp)
    window.removeEventListener('pointercancel', onColorSpectrumPointerUp)
  }

  function onColorSpectrumPointerUp(event: PointerEvent) {
    if (colorSpectrumActivePointerId !== event.pointerId) return
    stopColorSpectrumDragging()
  }

  function onColorSpectrumPointerDown(event: PointerEvent) {
    colorSpectrumActivePointerId = event.pointerId
    const target = event.currentTarget as HTMLElement | null
    target?.setPointerCapture?.(event.pointerId)
    updateColorFromSpectrumClient(event.clientX, event.clientY)
    window.addEventListener('pointermove', onColorSpectrumPointerMove)
    window.addEventListener('pointerup', onColorSpectrumPointerUp)
    window.addEventListener('pointercancel', onColorSpectrumPointerUp)
  }

  function openColorPopover(node: ColorNodeItem) {
    activeColorPopoverNodeId.value = node.id
    setColorPopoverHex(node.targetColor)
  }

  function closeColorPopover() {
    stopColorSpectrumDragging()
    activeColorPopoverNodeId.value = null
  }

  function toggleColorPopover(node: ColorNodeItem) {
    if (activeColorPopoverNodeId.value === node.id) {
      closeColorPopover()
      return
    }
    openColorPopover(node)
  }

  function onColorPopoverHexInput(event: Event) {
    const value = (event.target as HTMLInputElement | null)?.value ?? ''
    const raw = value.trim().toUpperCase()
    const display = raw ? (raw.startsWith('#') ? raw : `#${raw}`) : '#'
    colorPopoverHex.value = display.slice(0, 7)
    if (!/^#?[0-9A-F]{6}$/.test(raw)) return
    const normalized = normalizeHexColor(raw)
    const nodeId = activeColorPopoverNodeId.value
    if (nodeId === null) return
    const node = options.colorNodes.value.find(item => item.id === nodeId)
    if (node) node.targetColor = normalized
  }

  function commitColorPopoverHex() {
    const raw = colorPopoverHex.value.trim().toUpperCase()
    if (/^#?[0-9A-F]{6}$/.test(raw)) {
      setColorPopoverHex(raw)
      return
    }
    const nodeId = activeColorPopoverNodeId.value
    if (nodeId !== null) {
      const node = options.colorNodes.value.find(item => item.id === nodeId)
      if (node) {
        setColorPopoverHex(node.targetColor)
        return
      }
    }
    setColorPopoverHex('#A3E635')
  }

  function onColorNodeTargetColorChange(node: ColorNodeItem, event: Event) {
    const input = (event.target as HTMLInputElement | null)?.value ?? ''
    node.targetColor = normalizeHexColor(input || node.targetColor)
    if (activeColorPopoverNodeId.value === node.id) {
      setColorPopoverHex(node.targetColor)
    }
  }

  async function onColorNodeEyedropperClick(node: ColorNodeItem) {
    const EyeDropperCtor = getEyeDropperConstructor()
    if (!EyeDropperCtor) {
      options.message.warning('当前浏览器不支持吸管取色')
      return
    }
    try {
      const eyeDropper = new EyeDropperCtor()
      const result = await eyeDropper.open()
      const pickedHex = normalizeHexColor(result.sRGBHex || node.targetColor)
      node.targetColor = pickedHex
      if (activeColorPopoverNodeId.value === node.id) {
        setColorPopoverHex(pickedHex)
      }
    } catch (error) {
      if (error instanceof DOMException && error.name === 'AbortError') return
      options.message.error('吸管取色失败，请重试')
    }
  }

  onUnmounted(() => {
    stopColorSpectrumDragging()
  })

  return {
    activeColorPopoverNodeId,
    colorPopoverHex,
    colorPopoverHue,
    colorPopoverSaturation,
    colorPopoverLightness,
    colorSpectrumRef,
    colorSpectrumStyle,
    colorSpectrumCursorStyle,
    normalizeHexColor,
    setColorPopoverHex,
    applyColorPopoverToNode,
    onColorSpectrumPointerDown,
    openColorPopover,
    closeColorPopover,
    toggleColorPopover,
    onColorPopoverHexInput,
    commitColorPopoverHex,
    onColorNodeTargetColorChange,
    onColorNodeEyedropperClick,
  }
}
