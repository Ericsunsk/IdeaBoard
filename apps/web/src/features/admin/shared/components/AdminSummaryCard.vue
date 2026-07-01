<template>
  <Card :pt="adminCardPt">
    <template #subtitle>
      <span class="text-admin-body font-bold text-admin-subtitle-color tracking-normal">
        {{ label }}
      </span>
    </template>
    <template #title>
      <span class="text-admin-modal-title font-[780] text-admin-title-color leading-[1.05]">
        {{ value }}
      </span>
    </template>
    <template v-if="footer !== undefined" #content>{{ footer }}</template>
  </Card>
</template>

<script setup lang="ts">
import Card from 'primevue/card'
import { adminCardPt } from '../lib/adminCardPt'

defineProps<{
  label: string
  value: string | number
  footer?: string | number
}>()
</script>
