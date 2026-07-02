import { useAppToast } from '../../../shared/composables/useAppToast'
import { formatDisplayErrorMessage } from '../../../shared/utils/errorMessages'
import { unsupportedUploadImageMessage } from '../../../shared/utils/imageUploadSupport'

type AdminToast = Pick<ReturnType<typeof useAppToast>, 'success' | 'warning' | 'error'>

type AdminMessagePort = {
  success: (message: string) => void
  warning: (message: string) => void
  error: (message: string) => void
}

export type AdminFeedback = {
  login: {
    succeeded: () => void
    sessionRevokeFailed: () => void
    nonAdminAccount: () => void
    failed: (error: unknown) => void
  }
  channels: {
    providerConfig: AdminMessagePort
  }
  users: {
    management: AdminMessagePort
  }
  promptConfig: {
    loadPromptsFailed: (error: unknown) => void
    promptSaved: (featureTitle: string) => void
    savePromptFailed: (error: unknown) => void
  }
  systemSettings: {
    branding: Pick<AdminMessagePort, 'success' | 'error'>
    access: Pick<AdminMessagePort, 'success' | 'error'>
    taskMonitor: Pick<AdminMessagePort, 'success' | 'error'>
    publicURL: Pick<AdminMessagePort, 'success' | 'error'>
    mediaUpload: Pick<AdminMessagePort, 'success' | 'error'>
    unsupportedLogoFile: () => void
  }
  taskMonitor: {
    resultDetail: Pick<AdminMessagePort, 'success' | 'error'>
  }
}

export function createAdminFeedback(appToast: AdminToast): AdminFeedback {
  const message: AdminMessagePort = {
    success: (text) => appToast.success(text),
    warning: (text) => appToast.warning(text),
    error: (text) => appToast.error(text),
  }

  return {
    login: {
      succeeded() {
        appToast.success('管理员登录成功')
      },
      sessionRevokeFailed() {
        appToast.warning('会话撤销失败，请稍后重试')
      },
      nonAdminAccount() {
        appToast.error('该账号不是管理员')
      },
      failed(error) {
        appToast.error(formatDisplayErrorMessage(error, '管理员登录失败'))
      },
    },
    channels: {
      providerConfig: message,
    },
    users: {
      management: message,
    },
    promptConfig: {
      loadPromptsFailed(error) {
        appToast.error(formatDisplayErrorMessage(error, '加载提示词失败'))
      },
      promptSaved(featureTitle) {
        appToast.success(`「${featureTitle}」提示词已保存`)
      },
      savePromptFailed(error) {
        appToast.error(formatDisplayErrorMessage(error, '保存失败'))
      },
    },
    systemSettings: {
      branding: message,
      access: message,
      taskMonitor: message,
      publicURL: message,
      mediaUpload: message,
      unsupportedLogoFile() {
        appToast.error(unsupportedUploadImageMessage())
      },
    },
    taskMonitor: {
      resultDetail: message,
    },
  }
}

export function useAdminFeedback() {
  return createAdminFeedback(useAppToast())
}
