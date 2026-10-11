/**
 * P5 Canvas root seam (PR2: extends PR1 alias seam with page seam, no behavior change).
 *
 * Re-exports the three tool public indexes, the existing pure core/nodes
 * subpaths, plus the two page components per planned seam record.
 * Nothing here is new implementation: every symbol resolves to its current
 * canonical source. PR3 removes deprecated aliases.
 */
export * from './tools/retouch'
export * from './tools/inpaint'
export * from './tools/image-tools'
export * from './core/nodes/imageActions'
export * from './core/nodes/serviceLabelKey'
export { default as CanvasLandingPage } from './ui/pages/CanvasLandingPage.vue'
export { default as CanvasPage } from './ui/pages/CanvasPage.vue'
