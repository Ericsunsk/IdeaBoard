import type { Component } from 'vue'
import {
  Combine,
  Copy,
  Cut,
  EditPencil,
  Expand,
  Palette,
  Spark,
  ZoomIn,
} from '@iconoir/vue'
import { WORKFLOW_FEATURE } from './workflowFeatures'

// Workflow feature 到图标的单源：任务监控 Service 列与定价 Service 列共用，
// 新增 feature 只改这里（key 必须与 WORKFLOW_FEATURE slug 一致）。
const workflowFeatureIcons: Readonly<Record<string, Component>> = {
  [WORKFLOW_FEATURE.render]: Palette,
  [WORKFLOW_FEATURE.variations]: Copy,
  [WORKFLOW_FEATURE.inpaint]: EditPencil,
  [WORKFLOW_FEATURE.cutout]: Cut,
  [WORKFLOW_FEATURE.fusion]: Combine,
  [WORKFLOW_FEATURE.upscale]: ZoomIn,
  [WORKFLOW_FEATURE.outpaint]: Expand,
  [WORKFLOW_FEATURE.generation]: Spark,
}

export function getWorkflowFeatureIcon(slug: string): Component | undefined {
  if (!slug) return undefined
  return workflowFeatureIcons[slug] ?? workflowFeatureIcons[slug.toLowerCase()]
}
