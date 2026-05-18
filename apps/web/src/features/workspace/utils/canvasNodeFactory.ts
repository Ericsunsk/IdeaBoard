import {
  DEFAULT_COLOR_NODE_COLOR,
  DEFAULT_COLOR_STRENGTH,
  DEFAULT_IMAGE_NODE_OUTPUT_COUNT,
  DEFAULT_PROMPT_NODE_OUTPUT_COUNT,
  DEFAULT_PROMPT_ASPECT_RATIO,
  createDefaultModelAngleSpec,
  ENHANCE_EXTEND_PERCENT_DEFAULT,
  MODEL_REFERENCE_STRENGTH_DEFAULT,
  MODEL_POSE_STRENGTH_DEFAULT,
  WEAR_OPACITY_DEFAULT,
  WEAR_ROTATION_DEFAULT,
  WEAR_SCALE_DEFAULT,
  WEAR_TILE_MODE_DEFAULT,
  getDefaultCanvasNodeTitle,
} from '../types/canvasNodes'
import type {
  CanvasPoint,
  ColorNodeItem,
  EnhanceNodeItem,
  EditNodeItem,
  FusionNodeItem,
  ImageNodeItem,
  MaskNodeItem,
  ModelNodeItem,
  PromptNodeItem,
  WearNodeItem,
  ViewpointNodeItem,
} from '../types/canvasNodes'

function createCanvasEntityId(): number {
  return Date.now() + Math.random()
}

export function createPromptNodeAt(point: CanvasPoint): PromptNodeItem {
  return {
    id: createCanvasEntityId(),
    type: 'prompt',
    x: point.x,
    y: point.y,
    width: 280,
    title: getDefaultCanvasNodeTitle('prompt'),
    text: '',
    outputCount: DEFAULT_PROMPT_NODE_OUTPUT_COUNT,
    aspectRatio: DEFAULT_PROMPT_ASPECT_RATIO,
    inputImages: [],
  }
}

export function createModelNodeAt(point: CanvasPoint): ModelNodeItem {
  return {
    id: createCanvasEntityId(),
    type: 'model',
    x: point.x,
    y: point.y,
    width: 280,
    title: getDefaultCanvasNodeTitle('model'),
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
    title: getDefaultCanvasNodeTitle('viewpoint'),
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
    title: getDefaultCanvasNodeTitle('mask'),
    sourceImageUrl: '',
    sourceImageRatio: 1,
    executionMaskDataUrl: '',
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
    width: 280,
    title: getDefaultCanvasNodeTitle('image'),
    sourceImageUrl: '',
    sourceImageRatio: 1,
    outputCount: DEFAULT_IMAGE_NODE_OUTPUT_COUNT,
    outputStatus: 'idle',
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
    title: getDefaultCanvasNodeTitle('edit'),
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
    title: getDefaultCanvasNodeTitle('fusion'),
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
    title: getDefaultCanvasNodeTitle('enhance'),
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

export function createWearNodeAt(point: CanvasPoint): WearNodeItem {
  return {
    id: createCanvasEntityId(),
    type: 'wear',
    x: point.x,
    y: point.y,
    width: 286,
    title: getDefaultCanvasNodeTitle('wear'),
    tileMode: WEAR_TILE_MODE_DEFAULT,
    rotation: WEAR_ROTATION_DEFAULT,
    scale: WEAR_SCALE_DEFAULT,
    opacity: WEAR_OPACITY_DEFAULT,
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
    title: getDefaultCanvasNodeTitle('color'),
    colors: [DEFAULT_COLOR_NODE_COLOR],
    strength: DEFAULT_COLOR_STRENGTH,
    outputStatus: 'idle',
    outputImages: [],
  }
}
