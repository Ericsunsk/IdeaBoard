import {
  DEFAULT_FUSION_NODE_OUTPUT_COUNT,
  DEFAULT_IMAGE_NODE_OUTPUT_COUNT,
  DEFAULT_PROMPT_NODE_OUTPUT_COUNT,
  DEFAULT_PROMPT_ASPECT_RATIO,
  createDefaultModelAngleSpec,
  DEFAULT_FUSION_BLEND_POINT,
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
  EditNodeItem,
  FusionNodeItem,
  ImageNodeItem,
  ModelNodeItem,
  PromptNodeItem,
  WearNodeItem,
  ViewpointNodeItem,
} from '../types/canvasNodes'
import { EDIT_NODE_MODE } from '../types/editNodeModes'

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
    mode: EDIT_NODE_MODE.partialEdit,
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
    prompt: '',
    outputCount: DEFAULT_FUSION_NODE_OUTPUT_COUNT,
    aspectRatio: DEFAULT_PROMPT_ASPECT_RATIO,
    blendPoint: { ...DEFAULT_FUSION_BLEND_POINT },
    inputImages: [],
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
