import type { Service } from '../../../../core/contracts/imageServices'

/** Locale keys mirror slugs (`upscale` -> `canvas.features.upscale`). */
export function serviceLabelKey(slug: Service): string {
  const camel = slug.replace(/-([a-z])/g, (_, character: string) => character.toUpperCase())
  return `canvas.features.${camel}`
}
