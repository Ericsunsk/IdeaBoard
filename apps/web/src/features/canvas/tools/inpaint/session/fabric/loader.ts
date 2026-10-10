import type { MaskPainter, MaskPainterOptions } from '../../core/painter.ts'

/**
 * Lazy Fabric painter entrypoint.
 *
 * This module statically depends only on the narrow `painter.ts` contract
 * (types, erased at build). The Fabric implementation (`adapter.ts`, which
 * pulls the `fabric` package) is loaded through a dynamic `import()` so it
 * lands in its own lazy chunk: it is fetched only when an inpaint canvas
 * actually mounts, never with the Canvas page chunk.
 */
export function loadFabricMaskPainter(
  host: HTMLCanvasElement,
  options: MaskPainterOptions,
): Promise<MaskPainter> {
  return import('./adapter.ts').then((module) => module.createFabricMaskPainter(host, options))
}
