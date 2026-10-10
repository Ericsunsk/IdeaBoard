import type { ChartOptions } from '../lib/chartPrimitives'
import { resolveMotionTiming, type MotionIntent } from './motionPolicy'

type MotionTimingOptions = NonNullable<Parameters<typeof resolveMotionTiming>[1]>

export function resolveChartAnimation(
  intent: MotionIntent,
  options?: MotionTimingOptions,
): ChartOptions['animation'] {
  const timing = resolveMotionTiming(intent, options)
  return timing.durationMs > 0 ? { duration: timing.durationMs } : false
}
