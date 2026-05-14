import {
  normalizeViewpointSelectionByCategory,
  VIEWPOINT_LIGHTING_OPTIONS,
  ENHANCE_UPSCALE_FACTOR_OPTIONS,
} from '../components/nodes/config/nodeOptionConfig'
import { clampMaskRefineAmount } from '../services/flowMaskProcessing'
import type { EnhanceNodeItem, ViewpointNodeItem } from '../types/canvasNodes'
import {
  clampEnhanceExtendPercent,
  clampImageNodeOutputCount,
  clampModelAngleYaw,
  clampModelPoseStrength,
  clampModelReferenceStrength,
  clampWearOpacity,
  clampWearRotation,
  clampWearScale,
  normalizeWearTileMode,
} from '../types/canvasNodes'

export function patchFlowSnapshotParam(
  snapshotNode: Record<string, any>,
  payload: { key: string; value: unknown },
): Record<string, any> {
  const { key, value } = payload
  switch (key) {
    case 'title': {
      const nextTitle = String(value ?? '').trim()
      if (!nextTitle || snapshotNode.title === nextTitle) return snapshotNode
      return { ...snapshotNode, title: nextTitle }
    }
    case 'mode':
      if (snapshotNode.mode === value) return snapshotNode
      return { ...snapshotNode, mode: value }
    case 'outputCount': {
      const nextOutputCount = clampImageNodeOutputCount(value)
      if (snapshotNode.outputCount === nextOutputCount) return snapshotNode
      return { ...snapshotNode, outputCount: nextOutputCount }
    }
    case 'upscaleMode':
      if (snapshotNode.upscaleMode === value) return snapshotNode
      return { ...snapshotNode, upscaleMode: value }
    case 'upscaleFactor':
      if (ENHANCE_UPSCALE_FACTOR_OPTIONS.includes(value as EnhanceNodeItem['upscaleFactor'])) {
        if (snapshotNode.upscaleFactor === value) return snapshotNode
        return { ...snapshotNode, upscaleFactor: value }
      }
      return snapshotNode
    case 'outputFormat':
      if (snapshotNode.outputFormat === value) return snapshotNode
      return { ...snapshotNode, outputFormat: value }
    case 'outputQuality': {
      const nextOutputQuality = Math.max(30, Math.min(100, Math.round(Number(value) || 0)))
      if (snapshotNode.outputQuality === nextOutputQuality) return snapshotNode
      return { ...snapshotNode, outputQuality: nextOutputQuality }
    }
    case 'extendTop': {
      const nextExtendTop = clampEnhanceExtendPercent(value)
      if (snapshotNode.extendTop === nextExtendTop) return snapshotNode
      return { ...snapshotNode, extendTop: nextExtendTop }
    }
    case 'extendRight': {
      const nextExtendRight = clampEnhanceExtendPercent(value)
      if (snapshotNode.extendRight === nextExtendRight) return snapshotNode
      return { ...snapshotNode, extendRight: nextExtendRight }
    }
    case 'extendBottom': {
      const nextExtendBottom = clampEnhanceExtendPercent(value)
      if (snapshotNode.extendBottom === nextExtendBottom) return snapshotNode
      return { ...snapshotNode, extendBottom: nextExtendBottom }
    }
    case 'extendLeft': {
      const nextExtendLeft = clampEnhanceExtendPercent(value)
      if (snapshotNode.extendLeft === nextExtendLeft) return snapshotNode
      return { ...snapshotNode, extendLeft: nextExtendLeft }
    }
    case 'strength': {
      const nextStrength = Math.max(0, Math.min(100, Math.round(Number(value) || 0)))
      if (snapshotNode.strength === nextStrength) return snapshotNode
      return { ...snapshotNode, strength: nextStrength }
    }
    case 'blendRatio': {
      const nextBlendRatio = Math.max(0, Math.min(100, Math.round(Number(value) || 0)))
      if (snapshotNode.blendRatio === nextBlendRatio) return snapshotNode
      return { ...snapshotNode, blendRatio: nextBlendRatio }
    }
    case 'tileMode': {
      const nextTileMode = normalizeWearTileMode(value)
      if (snapshotNode.tileMode === nextTileMode) return snapshotNode
      return { ...snapshotNode, tileMode: nextTileMode }
    }
    case 'rotation': {
      const nextRotation = clampWearRotation(value)
      if (snapshotNode.rotation === nextRotation) return snapshotNode
      return { ...snapshotNode, rotation: nextRotation }
    }
    case 'scale': {
      const nextScale = clampWearScale(value)
      if (snapshotNode.scale === nextScale) return snapshotNode
      return { ...snapshotNode, scale: nextScale }
    }
    case 'opacity': {
      const nextOpacity = clampWearOpacity(value)
      if (snapshotNode.opacity === nextOpacity) return snapshotNode
      return { ...snapshotNode, opacity: nextOpacity }
    }
    case 'angleSpec.yaw': {
      const nextYaw = clampModelAngleYaw(value)
      const nextAngleSpec = { ...(snapshotNode.angleSpec ?? {}), yaw: nextYaw }
      const prevAngleSpec = snapshotNode.angleSpec ?? {}
      if (prevAngleSpec.yaw === nextYaw) return snapshotNode
      return { ...snapshotNode, angleSpec: nextAngleSpec }
    }
    case 'angleSpec.framing': {
      const prevAngleSpec = snapshotNode.angleSpec ?? {}
      if (prevAngleSpec.framing === value) return snapshotNode
      return { ...snapshotNode, angleSpec: { ...prevAngleSpec, framing: value } }
    }
    case 'modelStrength': {
      const nextModelStrength = clampModelReferenceStrength(value)
      if (snapshotNode.modelStrength === nextModelStrength) return snapshotNode
      return { ...snapshotNode, modelStrength: nextModelStrength }
    }
    case 'poseStrength': {
      const nextPoseStrength = clampModelPoseStrength(value)
      if (snapshotNode.poseStrength === nextPoseStrength) return snapshotNode
      return { ...snapshotNode, poseStrength: nextPoseStrength }
    }
    case 'background':
      if (snapshotNode.background === value) return snapshotNode
      return { ...snapshotNode, background: value }
    case 'activeCategory': {
      const category = value as ViewpointNodeItem['activeCategory']
      const nextSelectedViewpoint = normalizeViewpointSelectionByCategory(
        category,
        String(snapshotNode.selectedViewpoint ?? ''),
      )
      if (
        snapshotNode.activeCategory === category &&
        snapshotNode.selectedViewpoint === nextSelectedViewpoint
      ) {
        return snapshotNode
      }
      return { ...snapshotNode, activeCategory: category, selectedViewpoint: nextSelectedViewpoint }
    }
    case 'selectedViewpoint': {
      const nextSelectedViewpoint = String(value ?? '')
      if (snapshotNode.selectedViewpoint === nextSelectedViewpoint) return snapshotNode
      return { ...snapshotNode, selectedViewpoint: nextSelectedViewpoint }
    }
    case 'lighting': {
      const allowed = VIEWPOINT_LIGHTING_OPTIONS.some(
        (option: { value: string }) => option.value === value,
      )
      if (allowed) {
        if (snapshotNode.lighting === value) return snapshotNode
        return { ...snapshotNode, lighting: value }
      }
      return snapshotNode
    }
    case 'drawMode':
      if (value === 'paint' || value === 'erase') {
        if (snapshotNode.drawMode === value) return snapshotNode
        return { ...snapshotNode, drawMode: value }
      }
      return snapshotNode
    case 'brushSize': {
      const nextBrushSize = Math.max(4, Math.min(48, Math.round(Number(value) || 0)))
      if (snapshotNode.brushSize === nextBrushSize) return snapshotNode
      return { ...snapshotNode, brushSize: nextBrushSize }
    }
    case 'refineAmount': {
      const nextRefineAmount = clampMaskRefineAmount(Number(value) || 0)
      if (snapshotNode.refineAmount === nextRefineAmount) return snapshotNode
      return { ...snapshotNode, refineAmount: nextRefineAmount }
    }
    default:
      return snapshotNode
  }
}
