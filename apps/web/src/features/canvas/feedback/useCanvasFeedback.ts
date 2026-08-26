import { useAppToast } from '../../../shared/composables/useAppToast'
import { i18n } from '../../../shared/i18n'

type CanvasToast = Pick<
  ReturnType<typeof useAppToast>,
  'success' | 'warning' | 'error' | 'loading' | 'dismiss'
>

const insufficientCreditsToastId = 'canvas-insufficient-credits'

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
    insufficientCredits: () => void
  }
  imageTools: {
    succeeded: (message: string) => void
    warning: (message: string) => void
    actionUnavailable: (message: string) => void
  }
}

export function createCanvasFeedback(
  appToast: CanvasToast,
  openRecharge: () => void,
): CanvasFeedback {
  const success = (message: string) => {
    appToast.success(message)
  }
  const warning = (message: string) => {
    appToast.warning(message)
  }
  const insufficientCredits = () => {
    appToast.warning(i18n.global.t('billing.insufficientCredits'), {
      id: insufficientCreditsToastId,
      duration: 6000,
      action: {
        label: i18n.global.t('billing.rechargeNow'),
        onClick: () => {
          openRecharge()
          appToast.dismiss(insufficientCreditsToastId)
        },
      },
    })
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
    insufficientCredits,
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

export function useCanvasFeedback(openRecharge: () => void) {
  return createCanvasFeedback(useAppToast(), openRecharge)
}
