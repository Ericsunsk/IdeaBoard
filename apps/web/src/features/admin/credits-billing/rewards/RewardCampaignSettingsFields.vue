<template>
  <div class="contents">
    <AdminFormField label="展示名称" :for="`${idPrefix}-display-name`" required>
      <InputText
        :id="`${idPrefix}-display-name`"
        v-model="displayName"
        unstyled
        :class="ibFormInputClass"
      />
    </AdminFormField>
    <AdminFormField :label="noteLabel" :for="`${idPrefix}-note`">
      <InputText
        :id="`${idPrefix}-note`"
        v-model="internalNote"
        unstyled
        :class="ibFormInputClass"
      />
    </AdminFormField>
    <AdminFormField label="Idea 数量" :for="`${idPrefix}-credit`" required>
      <InputNumber
        v-model="creditAmount"
        :input-id="`${idPrefix}-credit`"
        :min="1"
        :use-grouping="false"
        fluid
      />
    </AdminFormField>
    <AdminFormField
      label="总兑换上限"
      :for="`${idPrefix}-cap`"
      :hint="redemptionCapHint"
      required
    >
      <InputNumber
        v-model="maxRedemptions"
        :input-id="`${idPrefix}-cap`"
        :min="minRedemptions"
        :use-grouping="false"
        fluid
      />
    </AdminFormField>
    <AdminFormField label="开始时间（Asia/Shanghai）" :for="`${idPrefix}-start`">
      <input
        :id="`${idPrefix}-start`"
        v-model="startsAt"
        type="datetime-local"
        :class="ibFormInputClass"
      />
    </AdminFormField>
    <AdminFormField
      v-slot="{ describedbyId, invalid }"
      label="结束时间（Asia/Shanghai）"
      :for="`${idPrefix}-expiry`"
      :error="scheduleError"
    >
      <input
        :id="`${idPrefix}-expiry`"
        v-model="expiresAt"
        type="datetime-local"
        :class="ibFormInputClass"
        :aria-describedby="describedbyId"
        :aria-invalid="invalid || undefined"
      />
    </AdminFormField>
  </div>
</template>

<script setup lang="ts">
import InputNumber from 'primevue/inputnumber'
import InputText from 'primevue/inputtext'
import { ibFormInputClass } from '../../../../shared/theme/formControls'
import AdminFormField from '../../shared/components/AdminFormField.vue'

withDefaults(
  defineProps<{
    idPrefix: string
    noteLabel?: string
    scheduleError?: string
    minRedemptions?: number
    redemptionCapHint?: string
  }>(),
  {
    noteLabel: '内部备注',
    scheduleError: '',
    minRedemptions: 1,
    redemptionCapHint: '',
  },
)

const displayName = defineModel<string>('displayName', { required: true })
const internalNote = defineModel<string>('internalNote', { required: true })
const creditAmount = defineModel<number>('creditAmount', { required: true })
const maxRedemptions = defineModel<number>('maxRedemptions', { required: true })
const startsAt = defineModel<string>('startsAt', { required: true })
const expiresAt = defineModel<string>('expiresAt', { required: true })
</script>
