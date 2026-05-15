<template>
  <n-config-provider :theme-overrides="themeOverrides" :locale="naiveLocale">
    <n-message-provider>
      <n-dialog-provider>
        <n-notification-provider>
          <router-view />
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
        </n-notification-provider>
      </n-dialog-provider>
    </n-message-provider>
  </n-config-provider>
</template>

<script setup lang="ts">
import { zhCN } from 'naive-ui'
import { Toaster } from 'vue-sonner'

const naiveLocale = zhCN
const toastOptions = {
  class: 'ideaboard-toast',
  closeButtonPosition: 'top-right' as const,
}

const themeOverrides = {
  common: {
    primaryColor: '#6366f1',
    primaryColorHover: '#818cf8',
    primaryColorPressed: '#4f46e5',
    primaryColorSuppl: '#6366f1',
    borderRadius: '8px',
  },
  Card: {
    borderRadius: '12px',
  },
  Table: {
    borderRadius: '12px',
  },
}
</script>

<style>
* {
  margin: 0;
  padding: 0;
  box-sizing: border-box;
}

html,
body {
  height: 100%;
  background: #f8f9fd;
  color: #202436;
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
  -webkit-font-smoothing: antialiased;
}

#app {
  width: 100%;
  min-height: 100vh;
}

a {
  color: inherit;
  text-decoration: none;
}

::-webkit-scrollbar {
  width: 6px;
  height: 6px;
}

::-webkit-scrollbar-track {
  background: #f8f9fd;
}

::-webkit-scrollbar-thumb {
  background: #c8d0e4;
  border-radius: 3px;
}
</style>
