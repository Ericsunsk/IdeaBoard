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
        <div class="flex gap-2">
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
            :label="isEditing ? '生成新 Code' : '重新生成'"
            type="button"
            severity="secondary"
            outlined
            @click="regenerate"
          />
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
        hint="上海时间；清除后永久有效"
      >
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
      </AdminFormField>

      <Message v-if="errorMessage" severity="error" :closable="false" role="alert">
        {{ errorMessage }}
      </Message>
    </form>

    <template #footer>
      <Button
        label="取消"
        type="button"
        severity="secondary"
        outlined
        :disabled="pending"
        @click="close"
      />
      <Button
        :label="isEditing ? '保存更改' : '创建兑换码'"
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
