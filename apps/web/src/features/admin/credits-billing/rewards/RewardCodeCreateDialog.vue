<template>
  <AdminDialogShell
    :visible="visible"
    header="创建兑换码"
    size="form"
    :busy="pending"
    @update:visible="emit('update:visible', $event)"
  >
    <form
      id="reward-code-create-form"
      class="flex flex-col gap-4"
      @submit.prevent="submit"
    >
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
          Code <span class="text-admin-danger" aria-hidden="true">*</span>
        </label>
        <div class="flex gap-2">
          <InputText
            id="reward-code-value"
            v-model="form.code"
            fluid
            maxlength="128"
            required
            aria-required="true"
            autocomplete="off"
            :spellcheck="false"
            class="min-w-0 flex-1 font-mono"
            :invalid="Boolean(shownErrors.code)"
            :aria-describedby="
              shownErrors.code
                ? 'reward-code-value-hint reward-code-value-error'
                : 'reward-code-value-hint'
            "
            @input="clearError('code')"
          />
          <Button label="重新生成" type="button" severity="secondary" outlined @click="regenerate" />
        </div>
        <p id="reward-code-value-hint" class="m-0 text-admin-caption text-admin-text-3">
          保存后仅能查看末四位
        </p>
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
          hint="0 表示无限"
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

      <AdminFormField label="过期时间" for="reward-code-expires-at" hint="留空表示永不过期">
        <InputText id="reward-code-expires-at" v-model="form.expiresAt" type="datetime-local" fluid />
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
        label="创建兑换码"
        type="submit"
        form="reward-code-create-form"
        :loading="pending"
        :disabled="pending"
      />
    </template>
  </AdminDialogShell>
</template>

<script setup lang="ts">
import { reactive, watch } from 'vue'
import Button from 'primevue/button'
import InputNumber from 'primevue/inputnumber'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import AdminDialogShell from '../../shared/components/AdminDialogShell.vue'
import AdminFormField from '../../shared/components/AdminFormField.vue'
import type { CreateRewardCodeInput } from './rewardCodeAdminResource'
import { generateRewardCode } from './rewardCodeGenerator'

type FieldName = 'displayName' | 'code' | 'creditAmount' | 'maxRedemptions'

const props = withDefaults(
  defineProps<{ visible: boolean; pending?: boolean; errorMessage?: string }>(),
  { pending: false, errorMessage: '' },
)
const emit = defineEmits<{
  'update:visible': [visible: boolean]
  submit: [input: CreateRewardCodeInput]
}>()

const maxCredits = 9_000_000_000_000_000
const form = reactive<CreateRewardCodeInput>(emptyForm())
const shownErrors = reactive<Record<FieldName, string>>({
  displayName: '',
  code: '',
  creditAmount: '',
  maxRedemptions: '',
})

watch(
  () => props.visible,
  (visible) => {
    if (visible) {
      Object.assign(form, emptyForm())
    } else {
      form.code = ''
      form.idempotencyKey = ''
    }
    clearErrors()
  },
)

function emptyForm(): CreateRewardCodeInput {
  return {
    displayName: '',
    code: generateRewardCode(),
    creditAmount: 100,
    maxRedemptions: 0,
    expiresAt: '',
    idempotencyKey: crypto.randomUUID(),
  }
}

function validate(): boolean {
  shownErrors.displayName = form.displayName.trim() ? '' : '请输入名称'
  shownErrors.code = form.code.trim() ? '' : '请输入 Code'
  shownErrors.creditAmount =
    Number.isSafeInteger(form.creditAmount) &&
    form.creditAmount >= 1 &&
    form.creditAmount <= maxCredits
      ? ''
      : '奖励 Idea 必须为正整数'
  shownErrors.maxRedemptions =
    Number.isSafeInteger(form.maxRedemptions) && form.maxRedemptions >= 0
      ? ''
      : '兑换上限必须为非负整数'
  return Object.values(shownErrors).every((value) => !value)
}

function submit() {
  if (props.pending) return
  if (!validate()) return
  emit('submit', { ...form })
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

function close() {
  if (!props.pending) emit('update:visible', false)
}
</script>
