<template>
  <AccountDialog
    v-if="isHostAllowed"
    :visible="visible"
    :initial-tab="initialTab"
    :payment-return="paymentReturn"
    @update:visible="setVisible"
  />
</template>

<script setup lang="ts">
import AccountDialog from './AccountDialog.vue'
import { useAccountOverlay } from '../../application/overlay/accountOverlay'

const overlay = useAccountOverlay()
const { allowed: isHostAllowed, initialTab, paymentReturn, visible } = overlay.viewModel
const { setVisible } = overlay.commands
</script>
