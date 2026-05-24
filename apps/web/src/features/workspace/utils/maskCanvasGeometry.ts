export type MaskCanvasPoint = {
  x: number
  y: number
}

export function getMaskCanvasPoint(
  clientX: number,
  clientY: number,
  canvas: HTMLCanvasElement,
): MaskCanvasPoint {
  const rect = canvas.getBoundingClientRect()
  if (!rect.width || !rect.height) return { x: 0, y: 0 }
  const x = ((clientX - rect.left) / rect.width) * canvas.width
  const y = ((clientY - rect.top) / rect.height) * canvas.height
  return {
    x: Math.max(0, Math.min(canvas.width, x)),
    y: Math.max(0, Math.min(canvas.height, y)),
  }
}
