<template>
  <IconoirProvider :icon-props="iconoirProps">
    <router-view />
  </IconoirProvider>
  <Toaster
    class="ideaboard-toaster"
    position="top-center"
    :visible-toasts="4"
    :close-button="false"
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
import { onMounted, onUnmounted, watch } from 'vue'
import { Toaster } from 'vue-sonner'
import { useSiteBranding } from '../shared/composables/useSiteBranding'

const { logoURL, projectName } = useSiteBranding()
const iconoirProps = {
  color: 'currentColor',
  width: '1.5em',
  height: '1.5em',
} as const

const toastOptions = {
  class: 'ideaboard-toast',
}

let appScrollTimer: number | undefined

const defaultSiteIcon = {
  href: '/favicon.svg',
  type: 'image/svg+xml',
}

function ensureSiteIconLink() {
  const existing = document.querySelector<HTMLLinkElement>('link[rel="icon"]')
  if (existing) return existing

  const link = document.createElement('link')
  link.rel = 'icon'
  document.head.appendChild(link)
  return link
}

function inferSiteIconType(href: string): string {
  const path = href.split(/[?#]/, 1)[0]?.toLowerCase() ?? ''
  if (path.endsWith('.svg')) return 'image/svg+xml'
  if (path.endsWith('.png')) return 'image/png'
  if (path.endsWith('.jpg') || path.endsWith('.jpeg')) return 'image/jpeg'
  if (path.endsWith('.webp')) return 'image/webp'
  if (path.endsWith('.ico')) return 'image/x-icon'
  return ''
}

function updateSiteIcon(nextLogoURL: string) {
  const icon = ensureSiteIconLink()
  const href = nextLogoURL || defaultSiteIcon.href
  const type = nextLogoURL ? inferSiteIconType(href) : defaultSiteIcon.type

  icon.href = href
  if (type) {
    icon.type = type
  } else {
    icon.removeAttribute('type')
  }
}

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

watch(
  projectName,
  (value) => {
    document.title = value
  },
  { immediate: true },
)

watch(
  logoURL,
  (value) => {
    updateSiteIcon(value)
  },
  { immediate: true },
)

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
