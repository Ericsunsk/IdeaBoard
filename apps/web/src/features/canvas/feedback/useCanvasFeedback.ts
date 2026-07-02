import { useAppToast } from '../../../shared/composables/useAppToast'

type CanvasToast = Pick<ReturnType<typeof useAppToast>, 'success' | 'warning' | 'error' | 'loading'>

type CanvasBoardSessionMessage = {
  warning: (message: string) => void
  success: (message: string) => void
  error: (message: string) => void
}

type CanvasGraphMessage = {
  success: (message: string) => unknown
  warning: (message: string) => unknown
  loading: CanvasToast['loading']
}

type CanvasTaskExecutionMessage = {
  success: (message: string) => void
  warning: (message: string) => void
}

type CanvasImageToolsMessage = {
  success: (message: string) => void
  warning: (message: string) => void
}

export type CanvasFeedback = {
  boardSession: {
    runtimeMessage: CanvasBoardSessionMessage
    warning: (message: string) => void
    saveSucceeded: (message: string) => void
    saveFailed: (message: string) => void
  }
  graph: {
    runtimeMessage: CanvasGraphMessage
    operationSucceeded: (message: string) => void
    operationWarning: (message: string) => void
    operationLoading: CanvasToast['loading']
  }
  taskExecution: {
    runtimeMessage: CanvasTaskExecutionMessage
    succeeded: (message: string) => void
    warning: (message: string) => void
  }
  imageTools: {
    runtimeMessage: CanvasImageToolsMessage
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
    boardSession: {
      ...boardSession,
      runtimeMessage: {
        warning: boardSession.warning,
        success: boardSession.saveSucceeded,
        error: boardSession.saveFailed,
      },
    },
    graph: {
      ...graph,
      runtimeMessage: {
        success: graph.operationSucceeded,
        warning: graph.operationWarning,
        loading: graph.operationLoading,
      },
    },
    taskExecution: {
      ...taskExecution,
      runtimeMessage: {
        success: taskExecution.succeeded,
        warning: taskExecution.warning,
      },
    },
    imageTools: {
      ...imageTools,
      runtimeMessage: {
        success: imageTools.succeeded,
        warning: imageTools.warning,
      },
    },
  }
}

export function useCanvasFeedback() {
  return createCanvasFeedback(useAppToast())
}
