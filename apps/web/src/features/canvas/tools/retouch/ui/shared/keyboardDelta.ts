/**
 * One-dimensional keyboard delta for horizontal slider-like controls.
 * Shared by HistogramPlot markers and the two-handle slider so arrow-key
 * semantics stay identical. CurveGraph keeps its own two-dimensional
 * variant (input/output) and intentionally does not use this helper.
 */
export function resolveKeyboardDelta(key: string, step: number): number | null {
  switch (key) {
    case 'ArrowLeft':
    case 'ArrowDown':
      return -step
    case 'ArrowRight':
    case 'ArrowUp':
      return step
    default:
      return null
  }
}
