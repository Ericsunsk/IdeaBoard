<template>
  <AdminDialogShell
    :visible="visible"
    :header="isEditing ? t('admin.rewardCode.editTitle') : t('admin.rewardCode.createTitle')"
    :description="t('admin.rewardCode.description')"
    size="form"
    :busy="pending"
    @update:visible="emit('update:visible', $event)"
  >
    <form id="reward-code-form" class="flex flex-col gap-4" @submit.prevent="submit">
      <AdminFormField
        v-slot="{ describedbyId, invalid }"
        :label="t('admin.rewardCode.nameLabel')"
        for="reward-code-name"
        required
        :error="form.errors.value.displayName"
      >
        <AdminInput
          id="reward-code-name"
          v-model="form.values.displayName"
          maxlength="120"
          required
          aria-required="true"
          :is-invalid="invalid"
          :aria-describedby="describedbyId"
          @input="form.clearErrors('displayName')"
        />
      </AdminFormField>

      <div class="flex min-w-0 flex-col gap-1.5">
        <label
          for="reward-code-value"
          class="text-admin-meta font-admin-emphasis text-muted-foreground"
        >
          {{ t('admin.rewardCode.codeLabel') }}
          <span v-if="!isEditing" class="text-destructive" aria-hidden="true">*</span>
        </label>
        <div class="flex items-stretch gap-2">
          <AdminInput
            id="reward-code-value"
            v-model="form.values.code"
            maxlength="128"
            :required="!isEditing"
            :aria-required="!isEditing"
            autocomplete="off"
            :spellcheck="false"
            class="min-w-0 flex-1 font-mono"
            :is-invalid="Boolean(form.errors.value.code)"
            :aria-describedby="codeDescribedby"
            @input="form.clearErrors('code')"
          />
          <AdminButton
            variant="outline"
            class="aspect-square shrink-0 !rounded-control !p-0 flex items-center justify-center"
            :disabled="pending"
            :aria-label="
              isEditing ? t('admin.rewardCode.generateNew') : t('admin.rewardCode.regenerate')
            "
            :title="
              isEditing ? t('admin.rewardCode.generateNew') : t('admin.rewardCode.regenerate')
            "
            type="button"
            @click="regenerate"
          >
            <Shuffle class="size-4" />
          </AdminButton>
        </div>
        <p id="reward-code-value-hint" class="m-0 text-admin-caption text-muted-foreground">
          {{
            isEditing ? t('admin.rewardCode.codeHintEdit') : t('admin.rewardCode.codeHintCreate')
          }}
        </p>
        <AdminAlert
          v-if="isEditing && form.values.code.trim()"
          id="reward-code-rotation-warning"
          severity="warn"
        >
          {{ t('admin.rewardCode.rotationWarning') }}
        </AdminAlert>
        <AdminAlert v-if="form.errors.value.code" id="reward-code-value-error" severity="error">
          {{ form.errors.value.code }}
        </AdminAlert>
      </div>

      <div class="grid gap-4 sm:grid-cols-2">
        <AdminFormField
          v-slot="{ describedbyId, invalid }"
          :label="t('admin.rewardCode.creditAmountLabel')"
          for="reward-code-credit-amount"
          required
          :error="form.errors.value.creditAmount"
        >
          <AdminNumberInput
            id="reward-code-credit-amount"
            v-model="form.values.creditAmount"
            required
            aria-required="true"
            :min="1"
            :max="maxCredits"
            :use-grouping="true"
            :invalid="invalid"
            :aria-describedby="describedbyId"
            @update:model-value="form.clearErrors('creditAmount')"
          />
        </AdminFormField>
        <AdminFormField
          v-slot="{ describedbyId, invalid }"
          :label="t('admin.rewardCode.maxRedemptionsLabel')"
          for="reward-code-cap"
          required
          :hint="capHint"
          :error="form.errors.value.maxRedemptions"
        >
          <AdminNumberInput
            id="reward-code-cap"
            v-model="form.values.maxRedemptions"
            required
            aria-required="true"
            :min="0"
            :use-grouping="true"
            :invalid="invalid"
            :aria-describedby="describedbyId"
            @update:model-value="form.clearErrors('maxRedemptions')"
          />
        </AdminFormField>
      </div>

      <AdminFormField
        v-slot="{ describedbyId }"
        :label="t('admin.rewardCode.expiresAtLabel')"
        for="reward-code-expires-at"
        :hint="t('admin.rewardCode.expiresAtHint')"
      >
        <div class="flex flex-col gap-2">
          <div class="flex flex-wrap items-center gap-1.5">
            <button
              v-for="preset in expirationPresets"
              :key="preset.label"
              type="button"
              class="rounded-control px-2.5 py-1 text-xs font-semibold transition-colors duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)]"
              :class="
                isPresetActive(preset.days)
                  ? 'bg-primary text-primary-foreground shadow-xs'
                  : 'border border-border bg-card text-muted-foreground hover:bg-muted hover:text-foreground'
              "
              :disabled="pending"
              @click="applyPreset(preset.days)"
            >
              {{ preset.label }}
            </button>
          </div>

          <AdminInput
            id="reward-code-expires-at"
            v-model="form.values.expiresAt"
            type="datetime-local"
            :placeholder="t('admin.rewardCode.permanent')"
            :disabled="pending"
            :aria-describedby="describedbyId"
          />
        </div>
      </AdminFormField>

      <AdminAlert v-if="errorMessage" severity="error">
        {{ errorMessage }}
      </AdminAlert>
    </form>

    <template #footer>
      <AdminDialogActions
        :cancel-label="t('admin.common.cancel')"
        :confirm-label="
          isEditing ? t('admin.common.saveChanges') : t('admin.rewardCode.createTitle')
        "
        :loading="pending"
        :disabled="pending"
        :cancel-disabled="pending"
        @cancel="close"
        @confirm="submit"
      />
    </template>
  </AdminDialogShell>
</template>

<script setup lang="ts">
import { computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Shuffle } from '@iconoir/vue'
import * as v from 'valibot'
import { useForm } from '@/components/ui/form'
import { AdminAlert } from '../../../shared'
import { AdminButton } from '../../../shared'
import { AdminDialogActions } from '../../../shared'
import { AdminDialogShell } from '../../../shared'
import { AdminFormField } from '../../../shared'
import { AdminInput } from '../../../shared'
import { AdminNumberInput } from '../../../shared'
import { createUniqueId } from '../../../../../core/utils/ids'
import type { RewardCode, UpdateRewardCodeInput } from '../../session/rewards/resource'
import { generateRewardCode } from '../../core/rewards/rewardCodeGenerator'
import {
  isoToShanghaiDateTimeInput,
  shanghaiDateTimeInputToPickerDate,
  shanghaiEndOfDayPlusDays,
} from '../../core/rewards/rewardCodeTime'

type RewardCodeFormValue = Omit<UpdateRewardCodeInput, 'creditAmount' | 'maxRedemptions'> & {
  code: string
  idempotencyKey: string
  creditAmount: number | null
  maxRedemptions: number | null
}

export type RewardCodeFormSubmit = UpdateRewardCodeInput & {
  code: string
  idempotencyKey: string
}

const props = withDefaults(
  defineProps<{
    visible: boolean
    rewardCode?: RewardCode | null
    pending?: boolean
    errorMessage?: string
  }>(),
  { rewardCode: null, pending: false, errorMessage: '' },
)
const emit = defineEmits<{
  'update:visible': [visible: boolean]
  submit: [input: RewardCodeFormSubmit]
}>()

const { t } = useI18n()

const maxCredits = 9_000_000_000_000_000
const isEditing = computed(() => props.rewardCode != null)

const expirationPresets = computed(() => [
  { label: t('admin.rewardCode.presetPermanent'), days: 0 },
  { label: t('admin.rewardCode.preset7Days'), days: 7 },
  { label: t('admin.rewardCode.preset30Days'), days: 30 },
  { label: t('admin.rewardCode.preset90Days'), days: 90 },
  { label: t('admin.rewardCode.preset1Year'), days: 365 },
])

function emptyCreateForm(): RewardCodeFormValue {
  return {
    displayName: '',
    code: generateRewardCode(),
    creditAmount: 100,
    maxRedemptions: 0,
    expiresAt: '',
    idempotencyKey: createUniqueId(),
  }
}

const rewardCodeSchema = computed(() => {
  const isEdit = isEditing.value
  const minimumCap = props.rewardCode?.redemptionCount ?? 0

  return v.object({
    displayName: v.pipe(
      v.string(),
      v.trim(),
      v.minLength(1, t('admin.rewardCode.validation.nameRequired')),
      v.maxLength(120, t('admin.rewardCode.validation.nameMaxLength')),
    ),
    code: v.pipe(
      v.string(),
      v.trim(),
      v.check((val) => {
        if (!isEdit && val.length === 0) return false
        return true
      }, t('admin.rewardCode.validation.codeRequired')),
      v.check((val) => {
        if (val.length > 0 && val.length < 8) return false
        return true
      }, t('admin.rewardCode.validation.codeMinLength')),
    ),
    creditAmount: v.pipe(
      v.union([v.number(), v.null()]),
      v.check(
        (val) =>
          typeof val === 'number' && Number.isSafeInteger(val) && val >= 1 && val <= maxCredits,
        t('admin.rewardCode.validation.creditsPositive'),
      ),
    ),
    maxRedemptions: v.pipe(
      v.union([v.number(), v.null()]),
      v.check(
        (val) => typeof val === 'number' && Number.isSafeInteger(val) && val >= 0,
        t('admin.rewardCode.validation.capNonNegative'),
      ),
      v.check(
        (val) => {
          if (typeof val === 'number' && val > 0 && val < minimumCap) return false
          return true
        },
        t('admin.rewardCode.validation.capBelowCurrent', { count: minimumCap }),
      ),
    ),
    expiresAt: v.string(),
    idempotencyKey: v.string(),
  })
})

const form = useForm<RewardCodeFormValue>({
  schema: rewardCodeSchema,
  initialValues: emptyCreateForm(),
})

function applyPreset(days: number) {
  if (days === 0) {
    form.values.expiresAt = ''
    return
  }
  form.values.expiresAt = shanghaiEndOfDayPlusDays(days)
}

function isPresetActive(days: number): boolean {
  const current = shanghaiDateTimeInputToPickerDate(form.values.expiresAt)
  if (days === 0) {
    return !current
  }
  if (!current) return false
  const now = new Date()
  const diffMs = current.getTime() - now.getTime()
  const diffDays = Math.round(diffMs / (1000 * 60 * 60 * 24))
  return Math.abs(diffDays - days) <= 1
}

const codeDescribedby = computed(() =>
  [
    'reward-code-value-hint',
    isEditing.value && form.values.code.trim() ? 'reward-code-rotation-warning' : '',
    form.errors.value.code ? 'reward-code-value-error' : '',
  ]
    .filter(Boolean)
    .join(' '),
)

const capHint = computed(() => {
  if (!props.rewardCode) return t('admin.rewardCode.capHintZero')
  return t('admin.rewardCode.capHintMinimum', { count: props.rewardCode.redemptionCount })
})

watch(
  () => [props.visible, props.rewardCode] as const,
  ([visible]) => {
    if (visible) resetForm()
    else clearSensitiveFields()
  },
  { immediate: true },
)

function resetForm() {
  const rewardCode = props.rewardCode
  const target: RewardCodeFormValue = rewardCode
    ? {
        displayName: rewardCode.displayName,
        code: '',
        creditAmount: rewardCode.creditAmount,
        maxRedemptions: rewardCode.maxRedemptions,
        expiresAt: rewardCode.expiresAt ? isoToShanghaiDateTimeInput(rewardCode.expiresAt) : '',
        idempotencyKey: '',
      }
    : emptyCreateForm()
  form.resetForm(target)
}

const submit = form.handleSubmit((values) => {
  if (props.pending) return
  const { creditAmount, maxRedemptions } = values
  if (typeof creditAmount !== 'number' || typeof maxRedemptions !== 'number') return
  emit('submit', { ...values, code: values.code.trim(), creditAmount, maxRedemptions })
})

function regenerate() {
  form.values.code = generateRewardCode()
  form.clearErrors('code')
}

function clearSensitiveFields() {
  form.values.code = ''
  form.values.idempotencyKey = ''
  form.clearErrors()
}

function close() {
  if (!props.pending) emit('update:visible', false)
}
</script>
