// Outpaint core narrow sub-barrel (cycle-safe).
//
// Execution-layer modules (`session/execution/*`) and cross-tool math
// consumers must import geometry through here, never the tool root barrel:
// the root barrel re-exports the overlay component, which rides
// `session/execution` (billing quote), so execution -> root barrel would
// close a module cycle (no-circular).
export type { OutpaintRect } from './geometry.ts'
export {
  containImageRect,
  resolveOutpaintIntent,
  resolveOutpaintPixelGeometry,
} from './geometry.ts'
