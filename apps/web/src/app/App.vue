<template>
  <n-config-provider :theme="darkTheme" :theme-overrides="themeOverrides" :locale="naiveLocale">
    <n-message-provider
      :container-class="messageContainerClass"
      :theme-overrides="messageThemeOverrides"
    >
      <n-dialog-provider>
        <n-notification-provider>
          <router-view />
        </n-notification-provider>
      </n-dialog-provider>
    </n-message-provider>
  </n-config-provider>
</template>

<script setup lang="ts">
import { darkTheme, zhCN } from 'naive-ui'

const naiveLocale = zhCN
const route = useRoute()
const isUserRoute = computed(() => route.path.startsWith('/user'))
const messageContainerClass = computed(() => (isUserRoute.value ? 'user-message-container' : undefined))

const userMessageThemeOverrides = {
  borderRadius: '14px',
  closeBorderRadius: '999px',
  closeSize: '22px',
  closeIconSize: '16px',
  fontSize: '13px',
}
const messageThemeOverrides = computed(() => (isUserRoute.value ? userMessageThemeOverrides : undefined))

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
  }
}
</script>

<style>
* {
  margin: 0;
  padding: 0;
  box-sizing: border-box;
}

html, body {
  height: 100%;
  background: #18181b;
  color: #e4e4e7;
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
  background: #18181b;
}

::-webkit-scrollbar-thumb {
  background: #3f3f46;
  border-radius: 3px;
}
</style>
