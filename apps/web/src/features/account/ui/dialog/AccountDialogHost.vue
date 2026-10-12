<template>
  <AccountDialog
    v-if="isHostAllowed"
    :visible="isVisible"
    :initial-tab="initialTab"
    :payment-return="paymentReturn"
    @update:visible="setVisible"
  />
</template>

<script setup lang="ts">
import AccountDialog from './AccountDialog.vue'
import { useAccountOverlay } from '../../session/overlay'

const overlay = useAccountOverlay()
const { isAllowed: isHostAllowed, initialTab, paymentReturn, isVisible } = overlay.viewModel
const { setVisible } = overlay.commands
</script>
