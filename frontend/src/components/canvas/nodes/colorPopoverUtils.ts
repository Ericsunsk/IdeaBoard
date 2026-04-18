export function clampColorValue(value: number, min: number, max: number): number {
  return Math.min(max, Math.max(min, value))
}

export const DEFAULT_COLOR_HEX = '#A3E635'

export const DEFAULT_COLOR_SWATCHES = [
  DEFAULT_COLOR_HEX,
  '#7A6BD5',
  '#22C55E',
  '#3B82F6',
  '#F59E0B',
  '#EF4444',
  '#111827',
  '#F3F4F6',
]

export function hexToRgb(hex: string): { r: number; g: number; b: number } {
  const normalized = hex.replace('#', '')
  return {
    r: parseInt(normalized.slice(0, 2), 16),
    g: parseInt(normalized.slice(2, 4), 16),
    b: parseInt(normalized.slice(4, 6), 16),
  }
}

export function rgbToHex(r: number, g: number, b: number): string {
  const toHex = (num: number) => clampColorValue(Math.round(num), 0, 255).toString(16).padStart(2, '0')
  return `#${toHex(r)}${toHex(g)}${toHex(b)}`.toUpperCase()
}

export function rgbToHsv(r: number, g: number, b: number): { h: number; s: number; v: number } {
  const rn = r / 255
  const gn = g / 255
  const bn = b / 255
  const max = Math.max(rn, gn, bn)
  const min = Math.min(rn, gn, bn)
  const delta = max - min
  let h = 0
  let s = 0

  if (delta !== 0) {
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
  if (max !== 0) {
    s = delta / max
  }

  return {
    h: Math.round(h),
    s: Math.round(s * 100),
    v: Math.round(max * 100),
  }
}

export function hsvToRgb(h: number, s: number, v: number): { r: number; g: number; b: number } {
  const hn = ((h % 360) + 360) % 360 / 360
  const sn = clampColorValue(s, 0, 100) / 100
  const vn = clampColorValue(v, 0, 100) / 100
  const scaledHue = hn * 6
  const chroma = vn * sn
  const x = chroma * (1 - Math.abs((scaledHue % 2) - 1))
  const m = vn - chroma
  let rPrime = 0
  let gPrime = 0
  let bPrime = 0

  if (scaledHue >= 0 && scaledHue < 1) {
    rPrime = chroma
    gPrime = x
  } else if (scaledHue < 2) {
    rPrime = x
    gPrime = chroma
  } else if (scaledHue < 3) {
    gPrime = chroma
    bPrime = x
  } else if (scaledHue < 4) {
    gPrime = x
    bPrime = chroma
  } else if (scaledHue < 5) {
    rPrime = x
    bPrime = chroma
  } else {
    rPrime = chroma
    bPrime = x
  }

  return {
    r: Math.round((rPrime + m) * 255),
    g: Math.round((gPrime + m) * 255),
    b: Math.round((bPrime + m) * 255),
  }
}

export function normalizeHexColor(input: string): string {
  const raw = input.trim().toUpperCase()
  if (/^#[0-9A-F]{6}$/.test(raw)) return raw
  if (/^[0-9A-F]{6}$/.test(raw)) return `#${raw}`
  return DEFAULT_COLOR_HEX
}
