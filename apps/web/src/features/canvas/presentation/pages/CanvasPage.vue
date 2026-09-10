<template>
  <CanvasScreen
    :route="route"
    :router="router"
    :get-user-identity="getUserIdentity"
    :logout-and-redirect="logoutAndRedirect"
  />
</template>

<script setup lang="ts">
import { useRoute, useRouter } from 'vue-router'
import { useUserStore } from '../../../auth/session/userSession'
import { useLogoutRedirect } from '../../../../shared/composables/useLogoutRedirect'
import { CanvasScreen } from '../screens'

const route = useRoute()
const router = useRouter()
const userStore = useUserStore()
const { logoutAndRedirect } = useLogoutRedirect({ logout: () => userStore.logout() })

function stableLocalUserNumber(publicID?: string) {
  const value = publicID?.trim() || ''
  if (!value) return 0
  let hash = 2166136261
  for (let index = 0; index < value.length; index += 1) {
    hash ^= value.charCodeAt(index)
    hash = Math.imul(hash, 16777619)
  }
  return Math.abs(hash) || 1
}

function getUserIdentity() {
  return {
    userId: stableLocalUserNumber(userStore.user?.public_id),
    userUid: `${userStore.user?.storage_namespace ?? ''}`.trim(),
  }
}
</script>
