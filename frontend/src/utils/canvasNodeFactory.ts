import { DEFAULT_COLOR_NODE_COLOR, DEFAULT_COLOR_STRENGTH } from '../types/canvasNodes'
import type {
  CanvasImageItem,
  CanvasPoint,
  ColorNodeItem,
  EnhanceNodeItem,
  EditNodeItem,
  FusionNodeItem,
  FaceSwapNodeItem,
  ImageNodeItem,
  MannequinNodeItem,
  MaskNodeItem,
  ModelNodeItem,
  PromptNodeItem,
  GenerateNodeItem,
  TryOnNodeItem,
  ViewpointNodeItem,
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
    outputStatus: 'idle',
    outputImages: [],
  }
}

export function createMannequinNodeAt(point: CanvasPoint): MannequinNodeItem {
  return {
    id: createCanvasEntityId(),
    type: 'mannequin',
    x: point.x,
    y: point.y,
    width: 280,
    sourceImageUrl: '',
    sourceImageRatio: 1,
    outputStatus: 'idle',
    outputImages: [],
  }
}

export function createViewpointNodeAt(point: CanvasPoint): ViewpointNodeItem {
  return {
    id: createCanvasEntityId(),
    type: 'viewpoint',
    x: point.x,
    y: point.y,
    width: 320,
    sourceImages: [],
    activeCategory: 'top',
    selectedViewpoints: [],
    lighting: 'warm_white',
    outputStatus: 'idle',
    outputImages: [],
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

export function createEditNodeAt(point: CanvasPoint): EditNodeItem {
  return {
    id: createCanvasEntityId(),
    type: 'edit',
    x: point.x,
    y: point.y,
    width: 280,
    mode: 'partial-edit',
    outputStatus: 'idle',
    outputImages: [],
  }
}

export function createFusionNodeAt(point: CanvasPoint): FusionNodeItem {
  return {
    id: createCanvasEntityId(),
    type: 'fusion',
    x: point.x,
    y: point.y,
    width: 286,
    blendRatio: 50,
    outputStatus: 'idle',
    outputImages: [],
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
    outputStatus: 'idle',
    outputImages: [],
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
    outputStatus: 'idle',
    outputImages: [],
  }
}

export function createFaceSwapNodeAt(point: CanvasPoint): FaceSwapNodeItem {
  return {
    id: createCanvasEntityId(),
    type: 'faceswap',
    x: point.x,
    y: point.y,
    width: 286,
    outputStatus: 'idle',
    outputImages: [],
  }
}

export function createColorNodeAt(point: CanvasPoint): ColorNodeItem {
  return {
    id: createCanvasEntityId(),
    type: 'color',
    x: point.x,
    y: point.y,
    width: 286,
    colors: [DEFAULT_COLOR_NODE_COLOR],
    strength: DEFAULT_COLOR_STRENGTH,
    outputStatus: 'idle',
    outputImages: [],
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
    outputStatus: 'idle',
    outputImages: [],
  }
}
