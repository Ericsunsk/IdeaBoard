import { useAppToast } from '../../../shared/composables/useAppToast'

type CanvasToast = Pick<ReturnType<typeof useAppToast>, 'success' | 'warning' | 'error' | 'loading'>

export type CanvasFeedback = {
  boardSession: {
    warning: (message: string) => void
    saveSucceeded: (message: string) => void
    saveFailed: (message: string) => void
  }
  graph: {
    operationSucceeded: (message: string) => void
    operationWarning: (message: string) => void
    operationLoading: CanvasToast['loading']
  }
  taskExecution: {
    succeeded: (message: string) => void
    warning: (message: string) => void
  }
  imageTools: {
    succeeded: (message: string) => void
    warning: (message: string) => void
    actionUnavailable: (message: string) => void
  }
}

export function createCanvasFeedback(appToast: CanvasToast): CanvasFeedback {
  const success = (message: string) => {
    appToast.success(message)
  }
  const warning = (message: string) => {
    appToast.warning(message)
  }
  const error = (message: string) => {
    appToast.error(message)
  }

  const boardSession = {
    warning,
    saveSucceeded: success,
    saveFailed: error,
  }
  const graph = {
    operationSucceeded: success,
    operationWarning: warning,
    operationLoading: appToast.loading,
  }
  const taskExecution = {
    succeeded: success,
    warning,
  }
  const imageTools = {
    succeeded: success,
    warning,
    actionUnavailable: warning,
  }

  return {
    boardSession,
    graph,
    taskExecution,
    imageTools,
  }
}

export function useCanvasFeedback() {
  return createCanvasFeedback(useAppToast())
}
