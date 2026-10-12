// Retouch public barrel: the unique exit for image-tools / application seams.
// Partitions below are the minimal set commands + session need. core/*
// stays private (plan/recipe/raster helpers are imported deep by the
// retouch-internal apply / chrome-source modules, never through this barrel).

// — model 类型 —
export type {
  ImageRetouchKind,
  RetouchEffectLayer,
  RetouchEffectParamKey,
  RetouchEffectParamMap,
  RetouchHslParamKey,
  RetouchLayer,
  RetouchMode,
  RetouchNonHslWholeParams,
  RetouchParamValue,
} from './core/model.ts'

// — session / mode / coordinate 应用缝 —
export { useRetouchSession } from './session/useRetouchSession'
export { useRetouchMode } from './session/useRetouchMode'
export type { RetouchModeState } from './session/useRetouchMode'
export { useRetouchCoordinateAdapter } from './session/useRetouchCoordinateAdapter'
export type {
  RetouchChromeAnchorRect,
  RetouchImageRect,
} from './session/useRetouchCoordinateAdapter'
export type {
  ZeroCopyStageHandle,
  ZeroCopyStageStatus,
} from './session/stage/useZeroCopyRetouchStage.ts'

// — rendering 导出函数（session/runtime 拥有最终导出管线） —
export {
  cancelImageRetouchFinalRender,
  hasRenderableRetouchLayerStack,
  renderImageRetouchFile,
  renderImageRetouchRecipeFile,
  renderRetouchLayerStackFile,
} from './session/stage/runtime/finalRender'
export type { ImageRetouchRenderSource } from './session/stage/runtime/finalRender'

// — apply 管线（image-tools 经此唯一委托入口调用） —
export { createRetouchApplyCommands } from './session/applyRetouch'
export type {
  RetouchApplyBoardCapabilities,
  RetouchApplyFeedbackCapabilities,
  RetouchApplyScope,
  RetouchApplyScopeHost,
  RetouchApplySession,
  RetouchApplySource,
} from './session/applyRetouch'

// — chrome 源（stage/histogram 装配，image-tools 只做 mask+editor 合并） —
export { createRetouchChromeSource } from './session/chromeSource'
export type {
  RetouchChromeSource,
  RetouchChromeSourceOptions,
  RetouchEditorLayers,
} from './session/chromeSource'

// — runtime GPU 能力 —
export { isWebGLRetouchSupported } from './session/stage/runtime/webglEngine.ts'

// — chrome 别名 —
export { default as RetouchChrome } from './ui/chrome/Chrome.vue'
