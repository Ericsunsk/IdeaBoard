<template>
  <section
    class="flex flex-col gap-3.5 rounded-panel border border-border bg-card p-4.5 shadow-raised"
    :aria-label="t('admin.wallet.restrictionSectionAria')"
  >
    <div class="flex items-center justify-between border-b border-border pb-3">
      <h3 class="m-0 text-admin-title-sm font-bold text-foreground">
        {{ t('admin.wallet.restrictionSectionTitle') }}
      </h3>
      <span class="text-admin-caption text-muted-foreground">
        {{ t('admin.wallet.restrictionSourceNote') }}
      </span>
    </div>

    <div v-if="restrictions.length === 0" class="text-admin-body text-muted-foreground">
      {{ t('admin.wallet.restrictionEmpty') }}
    </div>
    <ul v-else class="flex flex-col gap-2">
      <li
        v-for="restriction in restrictions"
        :key="restriction.restriction_id"
        class="flex flex-wrap items-center justify-between gap-2 rounded-control border border-border bg-muted/50 px-3 py-2"
      >
        <div class="min-w-0">
          <span class="text-xs font-bold text-foreground">
            {{ restrictionLabel(restriction.type) }}
          </span>
          <AdminMono class="mt-0.5 block truncate text-admin-caption text-muted-foreground">
            {{ restriction.source }} · {{ relatedLabel(restriction.related_object) }}
          </AdminMono>
        </div>
        <AdminButton
          :label="t('admin.wallet.restrictionRelease')"
          size="sm"
          variant="ghost"
          :disabled="isRestrictionPending"
          :aria-label="t('admin.wallet.restrictionReleaseAria', { id: restriction.restriction_id })"
          @click="emit('release', restriction.restriction_id)"
        />
      </li>
    </ul>

    <form class="flex flex-col gap-3 border-t border-border pt-3" @submit.prevent="submitCreate">
      <div class="grid gap-3 sm:grid-cols-2">
        <AdminFormField
          v-slot="{ describedbyId, invalid }"
          :label="t('admin.wallet.restrictionTypeLabel')"
          for="admin-wallet-restriction-type"
          :error="hasFormErrors ? typeError : ''"
          required
        >
          <AdminSelect
            :model-value="restrictionType"
            input-id="admin-wallet-restriction-type"
            :options="typeOptions"
            :disabled="isRestrictionPending"
            :aria-describedby="describedbyId"
            :invalid="invalid"
            @update:model-value="onTypeChange"
          />
        </AdminFormField>
        <AdminFormField
          v-slot="{ describedbyId, invalid }"
          :label="t('admin.wallet.restrictionObjectTypeLabel')"
          for="admin-wallet-restriction-object-type"
          :error="hasFormErrors ? objectTypeError : ''"
          required
        >
          <AdminSelect
            :model-value="relatedObjectType"
            input-id="admin-wallet-restriction-object-type"
            :options="objectTypeOptions"
            :disabled="isRestrictionPending"
            :aria-describedby="describedbyId"
            :invalid="invalid"
            @update:model-value="onObjectTypeChange"
          />
        </AdminFormField>
      </div>
      <AdminFormField
        v-slot="{ describedbyId, invalid }"
        :label="t('admin.wallet.restrictionObjectIdLabel')"
        for="admin-wallet-restriction-object-id"
        :error="hasFormErrors ? objectIdError : ''"
        required
      >
        <AdminInput
          :model-value="relatedObjectId"
          input-id="admin-wallet-restriction-object-id"
          :placeholder="t('admin.wallet.restrictionObjectIdPlaceholder')"
          :disabled="isRestrictionPending"
          :aria-describedby="describedbyId"
          :invalid="invalid"
          @update:model-value="onObjectIdChange"
        />
      </AdminFormField>
      <div class="flex items-center justify-end gap-2.5">
        <ActionButton
          :label="t('admin.wallet.restrictionCreate')"
          type="submit"
          variant="default"
          :loading="isRestrictionPending"
          :disabled="isRestrictionPending"
        />
      </div>
    </form>
  </section>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import ActionButton from '../../../../../../core/components/ActionButton.vue'
import { AdminButton } from '../../../../shared'
import { AdminFormField } from '../../../../shared'
import { AdminInput } from '../../../../shared'
import { AdminMono } from '../../../../shared'
import { AdminSelect } from '../../../../shared'

type WalletRestrictionView = {
  restriction_id: string
  type: string
  source: string
  related_object: unknown
  reason: string
  created_at: string
}

export type WalletRestrictionDraft = {
  type: string
  relatedObjectType: string
  relatedObjectId: string
}

defineProps<{
  restrictions: readonly WalletRestrictionView[]
  isRestrictionPending: boolean
}>()

const emit = defineEmits<{
  release: [restrictionId: string]
  create: [draft: WalletRestrictionDraft]
}>()

const { t } = useI18n()

const restrictionType = ref('')
const relatedObjectType = ref('')
const relatedObjectId = ref('')
const hasFormErrors = ref(false)

const typeOptions = computed(() => [
  { label: t('admin.wallet.restrictionTypeSpending'), value: 'spending' },
  { label: t('admin.wallet.restrictionTypeLedgerIntegrity'), value: 'ledger_integrity' },
])
const objectTypeOptions = computed(() => [
  { label: t('admin.wallet.restrictionRelatedFinding'), value: 'finding' },
  { label: t('admin.wallet.restrictionRelatedReview'), value: 'control_review' },
])

const typeError = computed(() =>
  restrictionType.value ? '' : t('admin.wallet.restrictionTypeRequired'),
)
const objectTypeError = computed(() =>
  relatedObjectType.value ? '' : t('admin.wallet.restrictionObjectTypeRequired'),
)
const objectIdError = computed(() => {
  const trimmed = relatedObjectId.value.trim()
  if (!trimmed) return t('admin.wallet.restrictionObjectIdRequired')
  if (trimmed.length > 160) return t('admin.wallet.restrictionObjectIdTooLong')
  return ''
})

function restrictionLabel(type: string) {
  return type === 'ledger_integrity'
    ? t('admin.wallet.restrictionTypeLedgerIntegrity')
    : type === 'spending'
      ? t('admin.wallet.restrictionTypeSpending')
      : type
}

function relatedLabel(related: unknown) {
  if (related && typeof related === 'object') {
    const candidate = related as { type?: unknown; id?: unknown }
    if (typeof candidate.type === 'string' && typeof candidate.id === 'string') {
      return `${candidate.type} · ${candidate.id}`
    }
  }
  return t('admin.wallet.restrictionRelatedUnknown')
}

function onTypeChange(value: string | number | null) {
  restrictionType.value = typeof value === 'string' ? value : ''
  hasFormErrors.value = false
}

function onObjectTypeChange(value: string | number | null) {
  relatedObjectType.value = typeof value === 'string' ? value : ''
  hasFormErrors.value = false
}

function onObjectIdChange(value: string | number | null) {
  relatedObjectId.value = typeof value === 'string' ? value : ''
  hasFormErrors.value = false
}

function submitCreate() {
  hasFormErrors.value = true
  if (typeError.value || objectTypeError.value || objectIdError.value) return
  emit('create', {
    type: restrictionType.value,
    relatedObjectType: relatedObjectType.value,
    relatedObjectId: relatedObjectId.value.trim(),
  })
  restrictionType.value = ''
  relatedObjectType.value = ''
  relatedObjectId.value = ''
  hasFormErrors.value = false
}
</script>
