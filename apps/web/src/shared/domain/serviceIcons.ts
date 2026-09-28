import type { Component } from 'vue'
import { Combine, Copy, Cut, EditPencil, Expand, Palette, Spark, ZoomIn } from '@iconoir/vue'
import { SERVICE } from './imageServices'

// Workflow feature 到图标的单源：任务监控 Service 列与定价 Service 列共用，
// 新增 feature 只改这里（key 必须与 SERVICE slug 一致）。
const serviceIcons: Readonly<Record<string, Component>> = {
  [SERVICE.render]: Palette,
  [SERVICE.variations]: Copy,
  [SERVICE.inpaint]: EditPencil,
  [SERVICE.cutout]: Cut,
  [SERVICE.fusion]: Combine,
  [SERVICE.upscale]: ZoomIn,
  [SERVICE.outpaint]: Expand,
  [SERVICE.generation]: Spark,
}

export function getServiceIcon(slug: string): Component | undefined {
  if (!slug) return undefined
  return serviceIcons[slug] ?? serviceIcons[slug.toLowerCase()]
}
