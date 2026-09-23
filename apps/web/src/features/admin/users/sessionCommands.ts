import { useUserStore } from '../../auth/session/userSession'
import { impersonateAdminUser } from './api'

export type AdminUserRedirect = {
  schedule: (url: string, delayMs: number) => void
}

export type AdminUserSessionCommands = {
  startImpersonation: (
    publicId: string,
    options?: { operatorReason?: string | undefined },
  ) => Promise<void>
}

function createBrowserRedirect(): AdminUserRedirect {
  return {
    schedule(url, delayMs) {
      setTimeout(() => {
        window.location.href = url
      }, delayMs)
    },
  }
}

export function createAdminUserSessionCommands(
  redirect: AdminUserRedirect = createBrowserRedirect(),
): AdminUserSessionCommands {
  const userStore = useUserStore()

  return {
    async startImpersonation(publicId: string, options?: { operatorReason?: string | undefined }) {
      const operatorReason = options?.operatorReason?.trim() ?? ''
      if (!operatorReason) throw new Error('需要填写操作原因')
      await impersonateAdminUser(publicId, { operatorReason })

      userStore.clearAuth()
      redirect.schedule('/', 1000)
    },
  }
}
