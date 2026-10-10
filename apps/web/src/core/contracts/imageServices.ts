export const SERVICE = {
  generation: 'generation',
  inpaint: 'inpaint',
  cutout: 'cutout',
  fusion: 'fusion',
  render: 'render',
  variations: 'variations',
  upscale: 'upscale',
  outpaint: 'outpaint',
} as const

export type Service = (typeof SERVICE)[keyof typeof SERVICE]

export const SERVICE_SLUGS = Object.values(SERVICE) as readonly Service[]
