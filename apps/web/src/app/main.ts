import { createApp } from 'vue'
import { VueQueryPlugin } from '@tanstack/vue-query'
import { useAppToast } from '../shared/composables/useAppToast'
import { queryClient } from '../shared/query/queryClient'
import { i18n, initUserLocale } from '../shared/i18n'
import router from './router'
import App from './App.vue'
import { useUserStore } from '../features/auth'
import './style.css'
import 'vue-sonner/style.css'

await initUserLocale()

const app = createApp(App)
app.use(router)
app.use(i18n)
app.use(VueQueryPlugin, { queryClient })

// Last-resort containment: a render/computed crash must degrade to a toast,
// never a blank SPA. Duplicate bursts from a single broken render are
// suppressed; the handler itself must never throw.
const lastGlobalError = { message: '', at: 0 }
app.config.errorHandler = (error) => {
  console.error('[app] unhandled render error', error)
  try {
    const message = error instanceof Error ? error.message : String(error ?? '')
    const now = Date.now()
    if (message === lastGlobalError.message && now - lastGlobalError.at < 5_000) return
    lastGlobalError.message = message
    lastGlobalError.at = now
    useAppToast().error(i18n.global.t('common.errors.unexpected'))
  } catch {
    // Intentionally silent: error handling must not fail.
  }
}

const userStore = useUserStore()
void userStore.fetchUserInfo()

app.mount('#app')
