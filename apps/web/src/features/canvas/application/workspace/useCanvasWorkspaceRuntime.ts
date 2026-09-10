import {
  createCanvasWorkspaceRuntime,
  type UseCanvasWorkspaceRuntimeOptions,
} from './internal/createCanvasWorkspaceRuntimeParts'
import type { CanvasWorkspaceRuntime } from './model.ts'
import { provideCanvasBillingReadiness } from '../execution'

export function useCanvasWorkspaceRuntime(
  options: UseCanvasWorkspaceRuntimeOptions,
): CanvasWorkspaceRuntime {
  provideCanvasBillingReadiness()
  return createCanvasWorkspaceRuntime(options)
}
