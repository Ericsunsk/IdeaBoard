import { useUserStore } from '../../auth/session/authSession'
import { clearAuthScopedQueryCache } from '../../../shared/query/queryClient'
import { impersonateAdminUser } from './adminUserApi'

export type AdminUserRedirect = {
  schedule: (url: string, delayMs: number) => void
}

export type AdminUserSessionCommands = {
  startImpersonation: (publicId: string) => Promise<void>
}

export function createBrowserRedirect(): AdminUserRedirect {
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
    async startImpersonation(publicId: string) {
      await impersonateAdminUser(publicId)

      userStore.clearAuth()
      clearAuthScopedQueryCache()
      redirect.schedule('/', 1000)
    },
  }
}
