<template>
  <AccountManagementDialog
    v-if="isHostAllowed"
    :visible="visible"
    :initial-tab="initialTab"
    :payment-return="paymentReturn"
    @update:visible="setVisible"
  />
</template>

<script setup lang="ts">
import AccountManagementDialog from './AccountManagementDialog.vue'
import { useAccountManagementOverlay } from '../accountManagementOverlay'

const overlay = useAccountManagementOverlay()
const { allowed: isHostAllowed, initialTab, paymentReturn, visible } = overlay.viewModel
const { setVisible } = overlay.commands
</script>
