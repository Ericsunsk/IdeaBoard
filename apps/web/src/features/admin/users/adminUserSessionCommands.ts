import { useUserStore } from '../../../app/stores/user'
import { authClient, extractBetterAuthErrorMessage } from '../../../shared/auth/client'
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
      const { error } = await authClient.admin.impersonateUser({ userId: authUserId })
      if (error) {
        throw new Error(extractBetterAuthErrorMessage(error, '伪装失败'))
      }

      userStore.clearAuth()
      clearAuthScopedQueryCache()
      redirect.schedule('/', 1000)
    },
  }
}
