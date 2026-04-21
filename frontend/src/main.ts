import { createApp } from 'vue'
import { createPinia } from 'pinia'
import router from './router'
import App from './App.vue'
import { useUserStore } from './stores/user'
import './style.css'

const app = createApp(App)
const pinia = createPinia()
app.use(pinia)
app.use(router)

async function bootstrap() {
  const userStore = useUserStore(pinia)
  await userStore.fetchUserInfo(true)
  app.mount('#app')
}

void bootstrap()
