import { SERVICE } from './imageServices'

export const IMAGE_NODE_ACTION = {
  cutout: SERVICE.cutout,
  variations: SERVICE.variations,
  render: SERVICE.render,
  upscale: SERVICE.upscale,
  outpaint: SERVICE.outpaint,
} as const
