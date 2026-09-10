import { createApp } from 'vue'
import { VueQueryPlugin } from '@tanstack/vue-query'
import PrimeVue from 'primevue/config'
import ConfirmationService from 'primevue/confirmationservice'
import { queryClient } from '../shared/query/queryClient'
import {
  getActiveUserPrimeLocale,
  i18n,
  initUserLocale,
  registerPrimeVueConfig,
} from '../shared/i18n'
import { ideaBoardPrimePt } from './primePassThrough'
import { ideaBoardPrimePreset } from './primeTheme'
import router from './router'
import App from './App.vue'
import { useUserStore } from '../features/auth/session/userSession'
import './style.css'
import 'vue-sonner/style.css'

await initUserLocale()

const app = createApp(App)
app.use(router)
app.use(i18n)
app.use(VueQueryPlugin, { queryClient })
app.use(PrimeVue, {
  locale: getActiveUserPrimeLocale(),
  pt: ideaBoardPrimePt,
  ptOptions: {
    mergeSections: true,
    mergeProps: true,
  },
  theme: {
    preset: ideaBoardPrimePreset,
    options: {
      prefix: 'p',
      darkModeSelector: '.dark',
      cssLayer: false,
    },
  },
})
registerPrimeVueConfig(app.config.globalProperties.$primevue.config)
app.use(ConfirmationService)

const userStore = useUserStore()
void userStore.fetchUserInfo()

app.mount('#app')
