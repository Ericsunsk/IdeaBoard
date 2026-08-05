import { useUserStore, startImpersonation } from '../../auth/session/authSession'
import { clearAuthScopedQueryCache } from '../../../shared/query/queryClient'

export type AdminUserRedirect = {
  schedule: (url: string, delayMs: number) => void
}

export type AdminUserSessionCommands = {
  startImpersonation: (authUserId: string) => Promise<void>
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
    async startImpersonation(authUserId: string) {
      await startImpersonation(authUserId)

      userStore.clearAuth()
      clearAuthScopedQueryCache()
      redirect.schedule('/', 1000)
    },
  }
}
