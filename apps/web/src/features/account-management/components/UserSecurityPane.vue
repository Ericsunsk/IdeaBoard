<template>
  <AccountManagementPane title="账号安全">
    <form class="grid gap-5" novalidate @submit.prevent="handleUpdatePassword">
      <IbFormField
        v-slot="{
          ariaInvalid,
          describedbyId,
          fieldId,
          inlineError,
          inlineErrorText,
          invalid,
          required,
        }"
        label="当前原密码"
        :error="pwdErrors.oldPassword"
        field-id="profile-old-password"
        required
        inline-required
        :field-class="passwordFieldClass"
        :message-pt="ibFormErrorMessagePt"
      >
        <Password
          v-model="pwdForm.oldPassword"
          autocomplete="current-password"
          :feedback="false"
          fluid
          unstyled
          :pt="ibPasswordPt"
          :input-props="{
            id: fieldId,
            autocomplete: 'current-password',
            required,
            'aria-invalid': ariaInvalid,
            'aria-describedby': describedbyId,
          }"
          :invalid="invalid"
          :input-class="[passwordInputClass, inlineError ? passwordInputErrorClass : '']"
          :placeholder="inlineError ? inlineErrorText : '请输入当前使用的密码'"
          toggle-mask
          @blur="validatePasswordField('oldPassword')"
          @update:model-value="clearPasswordFieldError('oldPassword')"
        />
      </IbFormField>

      <div class="grid grid-cols-2 gap-5 max-[720px]:grid-cols-1">
        <IbFormField
          v-slot="{
            ariaInvalid,
            describedbyId,
            fieldId,
            inlineError,
            inlineErrorText,
            invalid,
            required,
          }"
          label="设置新密码"
          :error="pwdErrors.newPassword"
          field-id="profile-new-password"
          required
          inline-required
          :field-class="passwordFieldClass"
          :message-pt="ibFormErrorMessagePt"
        >
          <Password
            v-model="pwdForm.newPassword"
            autocomplete="new-password"
            :feedback="false"
            fluid
            unstyled
            :pt="ibPasswordPt"
            :input-props="{
              id: fieldId,
              autocomplete: 'new-password',
              required,
              'aria-invalid': ariaInvalid,
              'aria-describedby': describedbyId,
            }"
            :invalid="invalid"
            :input-class="[passwordInputClass, inlineError ? passwordInputErrorClass : '']"
            :placeholder="inlineError ? inlineErrorText : '不少于6位字符'"
            toggle-mask
            @blur="validatePasswordField('newPassword')"
            @update:model-value="handleNewPasswordInput"
          />
        </IbFormField>

        <IbFormField
          v-slot="{
            ariaInvalid,
            describedbyId,
            fieldId,
            inlineError,
            inlineErrorText,
            invalid,
            required,
          }"
          label="重复新密码"
          :error="pwdErrors.confirmPassword"
          field-id="profile-confirm-password"
          required
          inline-required
          :field-class="passwordFieldClass"
          :message-pt="ibFormErrorMessagePt"
        >
          <Password
            v-model="pwdForm.confirmPassword"
            autocomplete="new-password"
            :feedback="false"
            fluid
            unstyled
            :pt="ibPasswordPt"
            :input-props="{
              id: fieldId,
              autocomplete: 'new-password',
              required,
              'aria-invalid': ariaInvalid,
              'aria-describedby': describedbyId,
            }"
            :invalid="invalid"
            :input-class="[passwordInputClass, inlineError ? passwordInputErrorClass : '']"
            :placeholder="inlineError ? inlineErrorText : '请再次输入新密码'"
            toggle-mask
            @blur="validatePasswordField('confirmPassword')"
            @update:model-value="clearPasswordFieldError('confirmPassword')"
          />
        </IbFormField>
      </div>

      <Button
        unstyled
        type="submit"
        :label="pwdLoading ? '更新中...' : '确认更新密码'"
        :loading="pwdLoading"
        :disabled="pwdLoading"
        :class="accountPrimaryActionButtonClass"
        :pt="accountButtonPt"
      />
    </form>
  </AccountManagementPane>
</template>

<script setup lang="ts">
import Button from 'primevue/button'
import Password from 'primevue/password'
import {
  ibFormErrorMessagePt,
  ibFormFieldClass,
  ibFormInputClass,
  ibFormInputErrorClass,
  ibPasswordPt,
} from '../../../shared/theme/formControls'
import IbFormField from '../../../shared/components/IbFormField.vue'
import { usePasswordChangeWorkflow } from '../usePasswordChangeWorkflow'
import AccountManagementPane from './AccountManagementPane.vue'
import { accountButtonPt, accountPrimaryActionButtonClass } from './accountManagementUi'

const {
  form: pwdForm,
  errors: pwdErrors,
  loading: pwdLoading,
  validateField: validatePasswordField,
  clearFieldError: clearPasswordFieldError,
  handleNewPasswordInput,
  submit: handleUpdatePassword,
} = usePasswordChangeWorkflow()

const passwordFieldClass = ibFormFieldClass
const passwordInputClass = ibFormInputClass
const passwordInputErrorClass = ibFormInputErrorClass
</script>
