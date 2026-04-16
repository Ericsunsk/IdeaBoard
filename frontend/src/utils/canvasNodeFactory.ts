import type {
  CanvasImageItem,
  CanvasPoint,
  ColorNodeItem,
  EnhanceNodeItem,
  EditNodeItem,
  FusionNodeItem,
  ImageNodeItem,
  MaskNodeItem,
  ModelNodeItem,
  PaletteNodeItem,
  PromptNodeItem,
  GenerateNodeItem,
  TryOnNodeItem,
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

export function createEditNodeAt(point: CanvasPoint): EditNodeItem {
  return {
    id: createCanvasEntityId(),
    type: 'edit',
    x: point.x,
    y: point.y,
    width: 280,
    mode: 'partial-edit',
  }
}

export function createFusionNodeAt(point: CanvasPoint): FusionNodeItem {
  return {
    id: createCanvasEntityId(),
    type: 'fusion',
    x: point.x,
    y: point.y,
    width: 286,
    mode: 'style-fusion',
  }
}

export function createEnhanceNodeAt(point: CanvasPoint): EnhanceNodeItem {
  return {
    id: createCanvasEntityId(),
    type: 'enhance',
    x: point.x,
    y: point.y,
    width: 286,
    mode: 'pattern-upscale',
    upscaleMode: 'base',
    upscaleFactor: 2,
    outputFormat: 'jpg',
    outputQuality: 95,
  }
}

export function createTryOnNodeAt(point: CanvasPoint): TryOnNodeItem {
  return {
    id: createCanvasEntityId(),
    type: 'tryon',
    x: point.x,
    y: point.y,
    width: 286,
    mode: 'fabric-on-body',
    tileMode: 'seamless',
    rotation: 0,
    scale: 100,
    opacity: 100,
  }
}

export function createPaletteNodeAt(point: CanvasPoint): PaletteNodeItem {
  return {
    id: createCanvasEntityId(),
    type: 'palette',
    x: point.x,
    y: point.y,
    width: 286,
    mode: 'women',
    colors: ['#8B3A3A', '#6B2D3E'],
  }
}

export function createGenerateNodeAt(point: CanvasPoint): GenerateNodeItem {
  return {
    id: createCanvasEntityId(),
    type: 'generate',
    x: point.x,
    y: point.y,
    width: 286,
    mode: 'pattern-gen',
    styleModel: '',
    ratio: '1:1',
    count: 1,
    quality: '1k',
  }
}
