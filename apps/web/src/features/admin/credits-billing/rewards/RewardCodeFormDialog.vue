<template>
  <AdminDialogShell
    :visible="visible"
    :header="isEditing ? '编辑兑换码' : '创建兑换码'"
    size="form"
    :busy="pending"
    @update:visible="emit('update:visible', $event)"
  >
    <form id="reward-code-form" class="flex flex-col gap-4" @submit.prevent="submit">
      <AdminFormField
        v-slot="{ describedbyId, invalid }"
        label="名称"
        for="reward-code-name"
        required
        :error="shownErrors.displayName"
      >
        <InputText
          id="reward-code-name"
          v-model="form.displayName"
          fluid
          maxlength="120"
          required
          aria-required="true"
          :invalid="invalid"
          :aria-describedby="describedbyId"
          @input="clearError('displayName')"
        />
      </AdminFormField>

      <div class="flex min-w-0 flex-col gap-1.5">
        <label for="reward-code-value" class="text-admin-meta font-[680] text-admin-text-2">
          Code
          <span v-if="!isEditing" class="text-admin-danger" aria-hidden="true">*</span>
        </label>
        <div class="flex items-stretch gap-2">
          <InputText
            id="reward-code-value"
            v-model="form.code"
            fluid
            maxlength="128"
            :required="!isEditing"
            :aria-required="!isEditing"
            autocomplete="off"
            :spellcheck="false"
            class="min-w-0 flex-1 font-mono"
            :invalid="Boolean(shownErrors.code)"
            :aria-describedby="codeDescribedby"
            @input="clearError('code')"
          />
          <Button
            severity="contrast"
            outlined
            class="aspect-square shrink-0 !rounded-lg !p-0 flex items-center justify-center"
            :disabled="pending"
            :aria-label="isEditing ? '生成新 Code' : '重新生成'"
            :title="isEditing ? '生成新 Code' : '重新生成'"
            type="button"
            @click="regenerate"
          >
            <template #icon>
              <Shuffle class="size-4" />
            </template>
          </Button>
        </div>
        <p id="reward-code-value-hint" class="m-0 text-admin-caption text-admin-text-3">
          {{ isEditing ? '留空则保留当前 Code；填写或生成后将更换 Code' : '保存后仅能查看末四位' }}
        </p>
        <Message
          v-if="isEditing && form.code.trim()"
          id="reward-code-rotation-warning"
          severity="warn"
          size="small"
          :closable="false"
        >
          保存后旧 Code 将立即失效，新 Code 将立即生效。
        </Message>
        <Message
          v-if="shownErrors.code"
          id="reward-code-value-error"
          severity="error"
          size="small"
          :closable="false"
        >
          {{ shownErrors.code }}
        </Message>
      </div>

      <div class="grid gap-4 sm:grid-cols-2">
        <AdminFormField
          v-slot="{ describedbyId, invalid }"
          label="奖励 Idea"
          for="reward-code-credit-amount"
          required
          :error="shownErrors.creditAmount"
        >
          <InputNumber
            v-model="form.creditAmount"
            input-id="reward-code-credit-amount"
            fluid
            required
            aria-required="true"
            :min="1"
            :max="maxCredits"
            :use-grouping="true"
            :invalid="invalid"
            :aria-describedby="describedbyId"
            @update:model-value="clearError('creditAmount')"
          />
        </AdminFormField>
        <AdminFormField
          v-slot="{ describedbyId, invalid }"
          label="兑换上限"
          for="reward-code-cap"
          required
          :hint="capHint"
          :error="shownErrors.maxRedemptions"
        >
          <InputNumber
            v-model="form.maxRedemptions"
            input-id="reward-code-cap"
            fluid
            required
            aria-required="true"
            :min="0"
            :use-grouping="true"
            :invalid="invalid"
            :aria-describedby="describedbyId"
            @update:model-value="clearError('maxRedemptions')"
          />
        </AdminFormField>
      </div>

      <AdminFormField
        v-slot="{ describedbyId }"
        label="过期时间"
        for="reward-code-expires-at"
        hint="上海时间（UTC+8）；清除或留空为永久有效"
      >
        <div class="flex flex-col gap-2">
          <div class="flex flex-wrap items-center gap-1.5">
            <button
              v-for="preset in expirationPresets"
              :key="preset.label"
              type="button"
              class="rounded-lg px-2.5 py-1 text-xs font-semibold transition-colors"
              :class="
                isPresetActive(preset.days)
                  ? 'bg-admin-accent text-white shadow-xs'
                  : 'border border-admin-border-soft bg-admin-surface text-admin-text-2 hover:bg-admin-surface-hover hover:text-admin-text-1'
              "
              :disabled="pending"
              @click="applyPreset(preset.days)"
            >
              {{ preset.label }}
            </button>
          </div>

          <DatePicker
            v-model="expiresAtDate"
            input-id="reward-code-expires-at"
            date-format="yy-mm-dd"
            placeholder="永久有效"
            show-icon
            icon-display="input"
            show-time
            show-button-bar
            hour-format="24"
            :manual-input="false"
            :disabled="pending"
            :aria-describedby="describedbyId"
            :pt="adminDateTimePickerPt"
            unstyled
            fluid
          >
            <template #buttonbar="{ todayCallback, clearCallback }">
              <button type="button" :class="adminDateTimePickerActionClass" @click="todayCallback">
                今天
              </button>
              <button type="button" :class="adminDateTimePickerActionClass" @click="clearCallback">
                永久有效
              </button>
            </template>
          </DatePicker>
        </div>
      </AdminFormField>

      <Message v-if="errorMessage" severity="error" :closable="false" role="alert">
        {{ errorMessage }}
      </Message>
    </form>

    <template #footer>
      <Button
        label="取消"
        type="button"
        :class="adminActionButtonClass"
        severity="secondary"
        outlined
        :disabled="pending"
        @click="close"
      />
      <Button
        :label="isEditing ? '保存更改' : '创建兑换码'"
        :class="adminActionButtonClass"
        severity="contrast"
        type="submit"
        form="reward-code-form"
        :loading="pending"
        :disabled="pending"
      />
    </template>
  </AdminDialogShell>
</template>

<script setup lang="ts">
import { computed, reactive, watch } from 'vue'
import { Shuffle } from '@iconoir/vue'
import Button from 'primevue/button'
import DatePicker from 'primevue/datepicker'
import InputNumber from 'primevue/inputnumber'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import AdminDialogShell from '../../shared/components/AdminDialogShell.vue'
import AdminFormField from '../../shared/components/AdminFormField.vue'
import {
  adminDateTimePickerActionClass,
  adminDateTimePickerPt,
} from '../../shared/lib/adminDateTimePickerPt'
import { adminActionButtonClass } from '../../shared/lib/adminUiClasses'
import type { RewardCode, UpdateRewardCodeInput } from './rewardCodeAdminResource'
import { generateRewardCode } from './rewardCodeGenerator'
import {
  isoToShanghaiDateTimeInput,
  pickerDateToShanghaiDateTimeInput,
  shanghaiDateTimeInputToPickerDate,
} from './rewardCodeTime'

export type RewardCodeFormValue = UpdateRewardCodeInput & {
  code: string
  idempotencyKey: string
}

type FieldName = 'displayName' | 'code' | 'creditAmount' | 'maxRedemptions'

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
  submit: [input: RewardCodeFormValue]
}>()

const maxCredits = 9_000_000_000_000_000
const isEditing = computed(() => props.rewardCode != null)
const form = reactive<RewardCodeFormValue>(emptyCreateForm())
const expiresAtDate = computed<Date | null>({
  get: () => shanghaiDateTimeInputToPickerDate(form.expiresAt),
  set: (value) => {
    form.expiresAt = pickerDateToShanghaiDateTimeInput(value)
  },
})

const expirationPresets = [
  { label: '永久有效', days: 0 },
  { label: '7 天', days: 7 },
  { label: '30 天', days: 30 },
  { label: '90 天', days: 90 },
  { label: '1 年', days: 365 },
]

function applyPreset(days: number) {
  if (days === 0) {
    expiresAtDate.value = null
    return
  }
  const date = new Date()
  date.setDate(date.getDate() + days)
  date.setHours(23, 59, 0, 0)
  expiresAtDate.value = date
}

function isPresetActive(days: number): boolean {
  if (days === 0) {
    return !expiresAtDate.value
  }
  if (!expiresAtDate.value) return false
  const now = new Date()
  const diffMs = expiresAtDate.value.getTime() - now.getTime()
  const diffDays = Math.round(diffMs / (1000 * 60 * 60 * 24))
  return Math.abs(diffDays - days) <= 1
}
const shownErrors = reactive<Record<FieldName, string>>({
  displayName: '',
  code: '',
  creditAmount: '',
  maxRedemptions: '',
})
const codeDescribedby = computed(() =>
  [
    'reward-code-value-hint',
    isEditing.value && form.code.trim() ? 'reward-code-rotation-warning' : '',
    shownErrors.code ? 'reward-code-value-error' : '',
  ]
    .filter(Boolean)
    .join(' '),
)
const capHint = computed(() => {
  if (!props.rewardCode) return '0 表示无限'
  return `0 表示无限；当前已兑换 ${props.rewardCode.redemptionCount} 次`
})

watch(
  () => props.visible,
  (visible) => {
    if (visible) resetForm()
    else clearSensitiveFields()
    clearErrors()
  },
)

watch(
  () => props.rewardCode,
  () => {
    if (props.visible) resetForm()
  },
)

function emptyCreateForm(): RewardCodeFormValue {
  return {
    displayName: '',
    code: generateRewardCode(),
    creditAmount: 100,
    maxRedemptions: 0,
    expiresAt: '',
    idempotencyKey: crypto.randomUUID(),
  }
}

function resetForm() {
  const rewardCode = props.rewardCode
  Object.assign(
    form,
    rewardCode
      ? {
          displayName: rewardCode.displayName,
          code: '',
          creditAmount: rewardCode.creditAmount,
          maxRedemptions: rewardCode.maxRedemptions,
          expiresAt: rewardCode.expiresAt ? isoToShanghaiDateTimeInput(rewardCode.expiresAt) : '',
          idempotencyKey: '',
        }
      : emptyCreateForm(),
  )
}

function validate(): boolean {
  shownErrors.displayName = form.displayName.trim() ? '' : '请输入名称'
  const codeLength = form.code.trim().length
  shownErrors.code =
    !isEditing.value && codeLength === 0
      ? '请输入 Code'
      : codeLength > 0 && codeLength < 8
        ? 'Code 至少需要 8 个字符'
        : ''
  shownErrors.creditAmount =
    Number.isSafeInteger(form.creditAmount) &&
    form.creditAmount >= 1 &&
    form.creditAmount <= maxCredits
      ? ''
      : '奖励 Idea 必须为正整数'
  const minimumCap = props.rewardCode?.redemptionCount ?? 0
  shownErrors.maxRedemptions =
    !Number.isSafeInteger(form.maxRedemptions) || form.maxRedemptions < 0
      ? '兑换上限必须为非负整数'
      : form.maxRedemptions > 0 && form.maxRedemptions < minimumCap
        ? `兑换上限不能低于当前兑换次数 ${minimumCap}`
        : ''
  return Object.values(shownErrors).every((value) => !value)
}

function submit() {
  if (props.pending || !validate()) return
  emit('submit', { ...form, code: form.code.trim() })
}

function regenerate() {
  form.code = generateRewardCode()
  clearError('code')
}

function clearError(field: FieldName) {
  shownErrors[field] = ''
}

function clearErrors() {
  for (const field of Object.keys(shownErrors) as FieldName[]) shownErrors[field] = ''
}

function clearSensitiveFields() {
  form.code = ''
  form.idempotencyKey = ''
}

function close() {
  if (!props.pending) emit('update:visible', false)
}
</script>
