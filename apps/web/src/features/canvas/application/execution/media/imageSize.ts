import { readImageNaturalSize } from '../../../../../core/media/imageDimensions.ts'

/**
 * Canvas-local alias kept for existing execution callers.
 * Implementation lives in `shared/media/imageDimensions` (LRU + timeout +
 * in-flight dedupe); do not fork probe logic here.
 */
export function loadImageSizeFromUrl(
  url: string,
): Promise<{ width: number; height: number } | null> {
  return readImageNaturalSize(url)
}
