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

    <div class="my-8 border-t border-ib-border-soft" aria-hidden="true"></div>

    <form v-if="!emailOtpSent" class="grid gap-5" novalidate @submit.prevent="handleRequestEmailChange">
      <IbFormField
        v-slot="{ ariaInvalid, describedbyId, fieldId, inlineError, inlineErrorText, invalid, required }"
        label="当前密码（重新验证）"
        :error="emailErrors.currentPassword"
        field-id="email-current-password"
        required
        inline-required
        :field-class="passwordFieldClass"
        :message-pt="ibFormErrorMessagePt"
      >
        <Password
          v-model="emailForm.currentPassword"
          autocomplete="current-password"
          :feedback="false"
          fluid
          unstyled
          :pt="ibPasswordPt"
          :input-props="{ id: fieldId, autocomplete: 'current-password', required, 'aria-invalid': ariaInvalid, 'aria-describedby': describedbyId }"
          :invalid="invalid"
          :input-class="[passwordInputClass, inlineError ? passwordInputErrorClass : '']"
          :placeholder="inlineError ? inlineErrorText : '请输入当前密码'"
          toggle-mask
        />
      </IbFormField>
      <IbFormField
        v-slot="{ ariaInvalid, describedbyId, fieldId, inlineError, inlineErrorText, invalid, required }"
        label="新 Email"
        :error="emailErrors.newEmail"
        field-id="new-email"
        required
        inline-required
        :field-class="passwordFieldClass"
        :message-pt="ibFormErrorMessagePt"
      >
        <input
          :id="fieldId"
          v-model="emailForm.newEmail"
          type="email"
          autocomplete="email"
          :required="required"
          :class="[passwordInputClass, inlineError ? passwordInputErrorClass : '']"
          :invalid="invalid"
          :placeholder="inlineError ? inlineErrorText : '请输入新的 Email'"
          :aria-invalid="ariaInvalid"
          :aria-describedby="describedbyId"
        />
      </IbFormField>
      <Button
        unstyled
        type="submit"
        :label="emailLoading ? '发送中...' : '发送 Email 验证码'"
        :loading="emailLoading"
        :disabled="emailLoading"
        :class="accountPrimaryActionButtonClass"
        :pt="accountButtonPt"
      />
    </form>

    <form v-else class="grid gap-5" novalidate @submit.prevent="handleConfirmEmailChange">
      <p class="text-sm text-ib-text-subtle">验证码已发送到 {{ emailForm.newEmail }}。</p>
      <IbFormField
        v-slot="{ ariaInvalid, describedbyId, fieldId, inlineError, inlineErrorText, invalid, required }"
        label="Email 验证码"
        :error="emailErrors.otp"
        field-id="email-otp"
        required
        inline-required
        :field-class="passwordFieldClass"
        :message-pt="ibFormErrorMessagePt"
      >
        <input
          :id="fieldId"
          v-model="emailForm.otp"
          inputmode="numeric"
          autocomplete="one-time-code"
          maxlength="6"
          :required="required"
          :class="[passwordInputClass, inlineError ? passwordInputErrorClass : '']"
          :invalid="invalid"
          :placeholder="inlineError ? inlineErrorText : '输入 6 位验证码'"
          :aria-invalid="ariaInvalid"
          :aria-describedby="describedbyId"
        />
      </IbFormField>
      <Button
        unstyled
        type="submit"
        :label="emailLoading ? '更新中...' : '确认换绑 Email'"
        :loading="emailLoading"
        :disabled="emailLoading"
        :class="accountPrimaryActionButtonClass"
        :pt="accountButtonPt"
      />
    </form>
  </AccountManagementPane>
</template>

<script setup lang="ts">
import { ref } from 'vue'
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
import { useUserStore } from '../../../app/stores/user'
import { useAsyncAction } from '../../../shared/composables/useAsyncAction'
import { useAppToast } from '../../../shared/composables/useAppToast'
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
const userStore = useUserStore()
const { runAsyncAction } = useAsyncAction()
const appToast = useAppToast()
const emailLoading = ref(false)
const emailOtpSent = ref(false)
const emailForm = ref({ currentPassword: '', newEmail: '', otp: '' })
const emailErrors = ref({ currentPassword: '', newEmail: '', otp: '' })

async function handleRequestEmailChange() {
  emailErrors.value.currentPassword = emailForm.value.currentPassword
    ? ''
    : '请输入当前密码'
  emailErrors.value.newEmail = /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(emailForm.value.newEmail.trim())
    ? ''
    : '请输入有效的 Email'
  if (emailErrors.value.currentPassword || emailErrors.value.newEmail) return
  const sent = await runAsyncAction(
    async () => {
      await userStore.verifyCurrentPassword(emailForm.value.currentPassword)
      await userStore.requestEmailChangeOTP(emailForm.value.newEmail)
      return true
    },
    {
      loading: emailLoading,
      feedback: {
        success: () => appToast.success('验证码已发送'),
        error: (error) => appToast.error(error instanceof Error ? error.message : '发送失败'),
      },
    },
  )
  if (sent) emailOtpSent.value = true
}

async function handleConfirmEmailChange() {
  emailErrors.value.otp = /^\d{6}$/.test(emailForm.value.otp.trim())
    ? ''
    : '请输入 6 位验证码'
  if (emailErrors.value.otp) return
  const changed = await runAsyncAction(
    () => userStore.changeEmailWithOTP(emailForm.value.newEmail, emailForm.value.otp),
    {
      loading: emailLoading,
      feedback: {
        success: () => appToast.success('Email 已更新，其他会话已退出'),
        error: (error) => appToast.error(error instanceof Error ? error.message : 'Email 更新失败'),
      },
    },
  )
  if (changed) {
    emailOtpSent.value = false
    emailForm.value = { currentPassword: '', newEmail: '', otp: '' }
  }
}
</script>
