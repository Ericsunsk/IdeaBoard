<template>
  <AdminServiceCell
    :service="serviceOfSku(props.sku) ?? props.sku"
    :fallback-subtitle="props.namespace || props.sku"
    :fallback-icon="props.namespace === 'ai-search' ? Search : undefined"
  />
</template>

<script setup lang="ts">
import { Search } from '@iconoir/vue'
import { AdminServiceCell } from '../../../shared'
import { serviceOfSku } from '../../core/pricing/skuFeature'

const props = defineProps<{
  sku: string
  namespace: string
}>()
</script>
