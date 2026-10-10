import { nextTick } from 'vue'

/*
 * Shared instrument-handle element refs for retouch plots.
 * Converges the ref-map + set + nextTick-focus shape of HistogramPlot,
 * CurveGraph, and EditingTwoHandleSlider. Focus options stay per-caller so
 * CurveGraph keeps `{ preventScroll: true }` while HistogramPlot and the
 * two-handle slider keep their plain focus behavior unchanged. Stale-ref
 * pruning watches stay in their owning components.
 */

export function useInstrumentRefs() {
  const elementRefs = new Map<string, HTMLButtonElement>()

  function setInstrumentRef(id: string, element: unknown) {
    if (element instanceof HTMLButtonElement) {
      elementRefs.set(id, element)
      return
    }
    elementRefs.delete(id)
  }

  function focusInstrument(id: string, options?: FocusOptions) {
    void nextTick(() => {
      elementRefs.get(id)?.focus(options)
    })
  }

  return { elementRefs, setInstrumentRef, focusInstrument }
}
