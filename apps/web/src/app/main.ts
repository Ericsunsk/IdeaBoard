import { createApp } from 'vue'
import { VueQueryPlugin } from '@tanstack/vue-query'
import PrimeVue from 'primevue/config'
import ConfirmationService from 'primevue/confirmationservice'
import { queryClient } from '../shared/query/queryClient'
import { ideaBoardPrimePt } from './primePassThrough'
import { ideaBoardPrimePreset } from './primeTheme'
import router from './router'
import App from './App.vue'
import { useUserStore } from './stores/user'
import './style.css'
import 'vue-sonner/style.css'

const app = createApp(App)
app.use(router)
app.use(VueQueryPlugin, { queryClient })
app.use(PrimeVue, {
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
app.use(ConfirmationService)

const userStore = useUserStore()
void userStore.fetchUserInfo()

app.mount('#app')
