import type {
  CanvasImageItem,
  CanvasPoint,
  ColorNodeItem,
  ImageNodeItem,
  MaskNodeItem,
  ModelNodeItem,
  PromptNodeItem,
} from '../types/canvasNodes'

function createCanvasEntityId(): number {
  return Date.now() + Math.random()
}

export function createCanvasImageItemAt(
  point: CanvasPoint,
  url: string,
  heightRatio?: number,
): CanvasImageItem {
  return {
    id: createCanvasEntityId(),
    type: 'image',
    url,
    x: point.x,
    y: point.y,
    width: 300,
    height_ratio: heightRatio && heightRatio > 0 ? heightRatio : undefined,
  }
}

export function createPromptNodeAt(point: CanvasPoint): PromptNodeItem {
  return {
    id: createCanvasEntityId(),
    type: 'prompt',
    x: point.x,
    y: point.y,
    width: 280,
    text: '',
  }
}

export function createModelNodeAt(point: CanvasPoint): ModelNodeItem {
  return {
    id: createCanvasEntityId(),
    type: 'model',
    x: point.x,
    y: point.y,
    width: 280,
    sourceImageUrl: '',
    sourceImageRatio: 1,
    selectedAngles: ['front_full'],
    background: 'original',
  }
}

export function createMaskNodeAt(point: CanvasPoint): MaskNodeItem {
  return {
    id: createCanvasEntityId(),
    type: 'mask',
    x: point.x,
    y: point.y,
    width: 280,
    sourceImageUrl: '',
    sourceImageRatio: 1,
    maskDataUrl: '',
    brushSize: 18,
    drawMode: 'paint',
    refineAmount: 0,
    invertMask: false,
  }
}

export function createImageNodeAt(point: CanvasPoint): ImageNodeItem {
  return {
    id: createCanvasEntityId(),
    type: 'image-node',
    x: point.x,
    y: point.y,
    width: 286,
    sourceImageUrl: '',
    sourceImageRatio: 1,
    outputImages: [],
  }
}

export function createColorNodeAt(point: CanvasPoint): ColorNodeItem {
  return {
    id: createCanvasEntityId(),
    type: 'color',
    x: point.x,
    y: point.y,
    width: 276,
    targetColor: '#A3E635',
    strength: 70,
  }
}
