<template>
  <UserAuthScreen :mode="mode" />
</template>

<script setup lang="ts">
import UserAuthScreen from '../../features/auth/screens/UserAuthScreen.vue'

defineProps<{
  mode: 'login' | 'register'
}>()
</script>
