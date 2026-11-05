<template>
  <StatusBadge :tone="tone" :label="label" :title="hint" size="small" />
</template>

<script setup lang="ts">
// Member quota status (Org-only quota table). Every state keeps the
// StatusBadge carrier; a healthy capped quota renders as a textless neutral
// dot while problem states carry text. Copy resolves through the locale
// bundle (four-locale parity).
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import StatusBadge, { type AppStatusTone } from '@/core/components/StatusBadge.vue'
import { MEMBER_QUOTA_STATUS_KEYS } from './quotaKeys'
import type { MemberQuotaStatus } from './memberQuotaTableColumns'

const props = defineProps<{
  status: MemberQuotaStatus
}>()

const { t } = useI18n()

const statusKeys = MEMBER_QUOTA_STATUS_KEYS

const toneByStatus: Record<MemberQuotaStatus, AppStatusTone> = {
  normal: 'neutral',
  nearExhausted: 'warning',
  unlimited: 'neutral',
  exhausted: 'danger',
}

const tone = computed(() => toneByStatus[props.status])
const label = computed(() => (props.status === 'normal' ? undefined : t(statusKeys[props.status])))
const hint = computed(() =>
  props.status === 'nearExhausted' ? t(statusKeys.nearExhaustedHint) : undefined,
)
</script>
