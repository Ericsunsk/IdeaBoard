<template>
  <ProfileDialog v-if="isHostAllowed" :visible="visible" @update:visible="setVisible" />
</template>

<script setup lang="ts">
import { computed, watch } from 'vue'
import { useRoute } from 'vue-router'
import { useUserStore } from '../../../app/stores/user'
import {
  ADMIN_ROOT_PATH,
  USER_LOGIN_PATH,
  USER_REGISTER_PATH,
  isPathUnderRoute,
} from '../../../shared/constants/routePaths'
import ProfileDialog from './ProfileDialog.vue'
import { useProfileDialogController } from '../useProfileDialogController'

const route = useRoute()
const userStore = useUserStore()
const { visible, close, setVisible } = useProfileDialogController()

const isAuthRoute = computed(
  () =>
    isPathUnderRoute(route.path, USER_LOGIN_PATH) ||
    isPathUnderRoute(route.path, USER_REGISTER_PATH),
)
const isHostAllowed = computed(
  () =>
    Boolean(userStore.user) && !isPathUnderRoute(route.path, ADMIN_ROOT_PATH) && !isAuthRoute.value,
)

watch(
  () => route.fullPath,
  () => {
    close()
  },
)

watch(
  () => userStore.user,
  (user) => {
    if (!user) {
      close()
    }
  },
)

watch(
  () => [isHostAllowed.value, visible.value] as const,
  ([allowed, isVisible]) => {
    if (!allowed && isVisible) {
      close()
    }
  },
)
</script>
