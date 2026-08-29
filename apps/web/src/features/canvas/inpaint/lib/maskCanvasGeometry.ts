export type MaskCanvasPoint = {
  x: number
  y: number
}

export type MaskCanvasPointerProjection = (clientX: number, clientY: number) => MaskCanvasPoint

function createMaskCanvasPointProjector(
  canvas: HTMLCanvasElement,
  rect: Pick<DOMRect, 'left' | 'top' | 'width' | 'height'>,
): MaskCanvasPointerProjection {
  if (!rect.width || !rect.height) return () => ({ x: 0, y: 0 })
  const scaleX = canvas.width / rect.width
  const scaleY = canvas.height / rect.height
  return (clientX, clientY) => {
    const x = (clientX - rect.left) * scaleX
    const y = (clientY - rect.top) * scaleY
    return {
      x: Math.max(0, Math.min(canvas.width, x)),
      y: Math.max(0, Math.min(canvas.height, y)),
    }
  }
}

export function createMaskCanvasPointerProjection(
  canvas: HTMLCanvasElement,
): MaskCanvasPointerProjection {
  return createMaskCanvasPointProjector(canvas, canvas.getBoundingClientRect())
}
