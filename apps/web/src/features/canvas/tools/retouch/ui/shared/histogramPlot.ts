import { computed, type ComputedRef } from 'vue'
import { createRetouchHistogram, sampleHistogramPath } from '../../core/histogram.ts'
import { retouchChannelToPlotPercent } from '../../core/channelScale.ts'

/**
 * Shared histogram skeleton for Levels and Threshold controls.
 * Converges the RGB histogram path (two lines), the marker keyboard/guide
 * mapping, and the guard/resolve/emit marker-update shape. Marker mode stays
 * per-control: Levels uses the default 3-marker button mode while Threshold
 * keeps its single-marker range mode.
 */
export function useRgbHistogramPath(
  resolveSource: () => ImageData | null | undefined,
): ComputedRef<string> {
  return computed(() => {
    const histogram = createRetouchHistogram(resolveSource() ?? null, 'rgb')
    return sampleHistogramPath(histogram, 80)
  })
}

export function createHistogramMarkerBase() {
  return {
    keyboardStep: retouchChannelToPlotPercent(1),
    keyboardShiftStep: retouchChannelToPlotPercent(10),
    guideLine: true as const,
  }
}

export function createHistogramMarkerUpdater<NextParams>(
  isKnownMarker: (markerId: string) => boolean,
  resolveNextParams: (markerId: string, position: number) => NextParams,
  emitUpdate: (next: NextParams) => void,
): (markerId: string, position: number) => void {
  return (markerId, position) => {
    if (!isKnownMarker(markerId)) return
    const nextParams = resolveNextParams(markerId, position)
    emitUpdate(nextParams)
  }
}
