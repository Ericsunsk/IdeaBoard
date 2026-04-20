import {
  DEFAULT_COLOR_NODE_COLOR,
  DEFAULT_COLOR_STRENGTH,
  createDefaultModelAngleSpec,
  ENHANCE_EXTEND_PERCENT_DEFAULT,
  MODEL_REFERENCE_STRENGTH_DEFAULT,
  MODEL_POSE_STRENGTH_DEFAULT,
  TRYON_OPACITY_DEFAULT,
  TRYON_ROTATION_DEFAULT,
  TRYON_SCALE_DEFAULT,
  TRYON_TILE_MODE_DEFAULT,
} from '../types/canvasNodes'
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
  PromptNodeItem,
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
    angleSpec: createDefaultModelAngleSpec(),
    modelStrength: MODEL_REFERENCE_STRENGTH_DEFAULT,
    poseStrength: MODEL_POSE_STRENGTH_DEFAULT,
    background: 'original',
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
    activeCategory: 'top',
    selectedViewpoint: '',
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
    mode: 'hd-upscale',
    upscaleMode: 'base',
    upscaleFactor: 2,
    outputFormat: 'jpg',
    outputQuality: 95,
    extendTop: ENHANCE_EXTEND_PERCENT_DEFAULT,
    extendRight: ENHANCE_EXTEND_PERCENT_DEFAULT,
    extendBottom: ENHANCE_EXTEND_PERCENT_DEFAULT,
    extendLeft: ENHANCE_EXTEND_PERCENT_DEFAULT,
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
    tileMode: TRYON_TILE_MODE_DEFAULT,
    rotation: TRYON_ROTATION_DEFAULT,
    scale: TRYON_SCALE_DEFAULT,
    opacity: TRYON_OPACITY_DEFAULT,
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
