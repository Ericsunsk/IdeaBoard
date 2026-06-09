<template>
  <IconoirProvider :icon-props="iconoirProps">
    <router-view />
  </IconoirProvider>
  <Toaster
    class="ideaboard-toaster"
    position="top-center"
    :visible-toasts="4"
    :close-button="true"
    :rich-colors="false"
    :expand="true"
    :duration="2600"
    :gap="10"
    :offset="16"
    :toast-options="toastOptions"
  />
</template>

<script setup lang="ts">
import { IconoirProvider } from '@iconoir/vue'
import { Toaster } from 'vue-sonner'

const iconoirProps = {
  color: 'currentColor',
  width: '1.5em',
  height: '1.5em',
  'stroke-width': 1.5,
} as const

const toastOptions = {
  class: 'ideaboard-toast',
  closeButtonPosition: 'top-right' as const,
}

let appScrollTimer: number | undefined

function setAppScrollingClass(enabled: boolean) {
  document.documentElement.classList.toggle('app-scrolling', enabled)
}

function handleAppScroll() {
  setAppScrollingClass(true)
  if (appScrollTimer) window.clearTimeout(appScrollTimer)
  appScrollTimer = window.setTimeout(() => {
    setAppScrollingClass(false)
    appScrollTimer = undefined
  }, 700)
}

onMounted(() => {
  window.addEventListener('scroll', handleAppScroll, { passive: true })
})

onUnmounted(() => {
  window.removeEventListener('scroll', handleAppScroll)
  if (appScrollTimer) {
    window.clearTimeout(appScrollTimer)
    appScrollTimer = undefined
  }
  setAppScrollingClass(false)
})
</script>
