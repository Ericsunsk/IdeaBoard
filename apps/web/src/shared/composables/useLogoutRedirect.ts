import { useRouter } from 'vue-router'
import { useUserStore } from '../../app/stores/user'

export function useLogoutRedirect() {
  const router = useRouter()
  const userStore = useUserStore()

  async function logoutAndRedirect(path: string, options: { replace?: boolean } = {}) {
    if (!(await userStore.logout())) {
      return false
    }
    if (options.replace) {
      await router.replace(path)
    } else {
      await router.push(path)
    }
    return true
  }

  return {
    logoutAndRedirect,
  }
}
