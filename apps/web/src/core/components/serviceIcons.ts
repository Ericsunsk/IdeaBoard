import type { Component } from 'vue'
import {
  BorderInner,
  ChatLines,
  EditPencil,
  Expand,
  Keyframes,
  Palette,
  Spark,
  ZoomIn,
} from '@iconoir/vue'
import { SERVICE, type Service } from '../contracts/imageServices'

const serviceIcons: Readonly<Record<Service, Component>> = {
  [SERVICE.render]: Palette,
  [SERVICE.variations]: Spark,
  [SERVICE.inpaint]: EditPencil,
  [SERVICE.cutout]: BorderInner,
  [SERVICE.fusion]: Keyframes,
  [SERVICE.upscale]: ZoomIn,
  [SERVICE.outpaint]: Expand,
  [SERVICE.generation]: ChatLines,
}

export function getServiceIcon(slug: Service): Component
export function getServiceIcon(slug: string): Component | undefined
export function getServiceIcon(slug: string): Component | undefined {
  const normalizedSlug = slug.toLowerCase()
  return Object.hasOwn(serviceIcons, normalizedSlug)
    ? serviceIcons[normalizedSlug as Service]
    : undefined
}
