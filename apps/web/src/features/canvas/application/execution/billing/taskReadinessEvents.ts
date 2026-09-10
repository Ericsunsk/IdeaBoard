import type { SubmitTaskReadinessData } from '../../../../../shared/api/task'

export type CanvasTaskReadinessListener = (readiness: SubmitTaskReadinessData) => void

const listeners = new Set<CanvasTaskReadinessListener>()

export function publishCanvasTaskReadiness(readiness: SubmitTaskReadinessData): void {
  for (const listener of listeners) {
    try {
      listener(readiness)
    } catch (error) {
      console.error('[canvas-billing] readiness listener failed', { readiness, error })
    }
  }
}

export function subscribeCanvasTaskReadiness(listener: CanvasTaskReadinessListener): () => void {
  listeners.add(listener)
  return () => listeners.delete(listener)
}
