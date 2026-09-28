<template>
  <AdminSubNavigationLayout
    :items="navigationItems"
    :active-path="activeNavigationPath"
    @navigate="navigate($event as NavigationPath)"
  />
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import AdminSubNavigationLayout from '../shared/components/AdminSubNavigationLayout.vue'
import { SETTINGS_MESSAGES } from './pages/messages'
import {
  ADMIN_SETTINGS_GENERAL_PATH,
  ADMIN_SETTINGS_MAIL_PATH,
  ADMIN_SETTINGS_SECURITY_PATH,
  ADMIN_SETTINGS_STORAGE_PATH,
  isPathUnderRoute,
} from '../../../shared/constants/routePaths'

type NavigationPath =
  | typeof ADMIN_SETTINGS_GENERAL_PATH
  | typeof ADMIN_SETTINGS_STORAGE_PATH
  | typeof ADMIN_SETTINGS_MAIL_PATH
  | typeof ADMIN_SETTINGS_SECURITY_PATH

const navigationItems: Array<{ key: string; path: NavigationPath; label: string }> = [
  { key: 'general', path: ADMIN_SETTINGS_GENERAL_PATH, label: SETTINGS_MESSAGES.navGeneral },
  { key: 'storage', path: ADMIN_SETTINGS_STORAGE_PATH, label: SETTINGS_MESSAGES.navStorage },
  { key: 'mail', path: ADMIN_SETTINGS_MAIL_PATH, label: SETTINGS_MESSAGES.navMail },
  { key: 'security', path: ADMIN_SETTINGS_SECURITY_PATH, label: SETTINGS_MESSAGES.navSecurity },
]

const route = useRoute()
const router = useRouter()

const activeNavigationPath = computed<NavigationPath>(
  () =>
    navigationItems.find((item) => isPathUnderRoute(route.path, item.path))?.path ??
    ADMIN_SETTINGS_GENERAL_PATH,
)

function navigate(path: NavigationPath) {
  if (isPathUnderRoute(route.path, path)) return
  void router.push(path)
}
</script>
