export function clampColorValue(value: number, min: number, max: number): number {
  return Math.min(max, Math.max(min, value))
}

export const DEFAULT_COLOR_SWATCHES = [
  '#A3E635',
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

export function rgbToHsl(r: number, g: number, b: number): { h: number; s: number; l: number } {
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

export function hslToRgb(h: number, s: number, l: number): { r: number; g: number; b: number } {
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
