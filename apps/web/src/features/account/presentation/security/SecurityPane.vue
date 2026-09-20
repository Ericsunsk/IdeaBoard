<template>
  <AccountPane :title="t('account.tabs.security')">
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
        :label="t('account.security.currentPassword')"
        :error="pwdErrors.oldPassword"
        field-id="profile-old-password"
        required
        inline-required
        :field-class="passwordFieldClass"
      >
        <PasswordInput
          :id="fieldId"
          v-model="pwdForm.oldPassword"
          autocomplete="current-password"
          :aria-invalid="ariaInvalid"
          :aria-describedby="describedbyId"
          :required="required"
          :invalid="invalid"
          :input-class="[passwordInputClass, inlineError ? passwordInputErrorClass : '']"
          :placeholder="
            inlineError ? inlineErrorText : t('account.security.currentPasswordPlaceholder')
          "
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
          :label="t('account.security.newPassword')"
          :error="pwdErrors.newPassword"
          field-id="profile-new-password"
          required
          inline-required
          :field-class="passwordFieldClass"
        >
          <PasswordInput
            :id="fieldId"
            v-model="pwdForm.newPassword"
            autocomplete="new-password"
            :aria-invalid="ariaInvalid"
            :aria-describedby="describedbyId"
            :required="required"
            :invalid="invalid"
            :input-class="[passwordInputClass, inlineError ? passwordInputErrorClass : '']"
            :placeholder="
              inlineError ? inlineErrorText : t('account.security.newPasswordPlaceholder')
            "
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
          :label="t('account.security.confirmPassword')"
          :error="pwdErrors.confirmPassword"
          field-id="profile-confirm-password"
          required
          inline-required
          :field-class="passwordFieldClass"
        >
          <PasswordInput
            :id="fieldId"
            v-model="pwdForm.confirmPassword"
            autocomplete="new-password"
            :aria-invalid="ariaInvalid"
            :aria-describedby="describedbyId"
            :required="required"
            :invalid="invalid"
            :input-class="[passwordInputClass, inlineError ? passwordInputErrorClass : '']"
            :placeholder="
              inlineError ? inlineErrorText : t('account.security.confirmPasswordPlaceholder')
            "
            @blur="validatePasswordField('confirmPassword')"
            @update:model-value="clearPasswordFieldError('confirmPassword')"
          />
        </IbFormField>
      </div>

      <Button
        type="submit"
        :loading="pwdLoading"
        :disabled="pwdLoading"
        :class="accountPrimaryActionButtonClass"
        >{{
          pwdLoading ? t('account.security.updating') : t('account.security.updatePassword')
        }}</Button
      >
    </form>

    <div class="my-8 border-t border-ib-border-soft" aria-hidden="true"></div>

    <form
      v-if="!emailOtpSent"
      class="grid gap-5"
      novalidate
      @submit.prevent="handleRequestEmailChange"
    >
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
        :label="t('account.security.reauthCurrentPassword')"
        :error="emailErrors.currentPassword"
        field-id="email-current-password"
        required
        inline-required
        :field-class="passwordFieldClass"
      >
        <PasswordInput
          :id="fieldId"
          v-model="emailForm.currentPassword"
          autocomplete="current-password"
          :aria-invalid="ariaInvalid"
          :aria-describedby="describedbyId"
          :required="required"
          :invalid="invalid"
          :input-class="[passwordInputClass, inlineError ? passwordInputErrorClass : '']"
          :placeholder="
            inlineError ? inlineErrorText : t('account.security.currentPasswordPlaceholder')
          "
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
        :label="t('account.security.newEmail')"
        :error="emailErrors.newEmail"
        field-id="new-email"
        required
        inline-required
        :field-class="passwordFieldClass"
      >
        <Input
          :id="fieldId"
          v-model="emailForm.newEmail"
          type="email"
          autocomplete="email"
          :required="required"
          :class="[passwordInputClass, inlineError ? passwordInputErrorClass : '']"
          :invalid="invalid"
          :placeholder="inlineError ? inlineErrorText : t('account.security.newEmailPlaceholder')"
          :aria-invalid="ariaInvalid"
          :aria-describedby="describedbyId"
        />
      </IbFormField>
      <Button
        type="submit"
        :loading="emailLoading"
        :disabled="emailLoading"
        :class="accountPrimaryActionButtonClass"
        >{{ emailLoading ? t('account.security.sending') : t('account.security.sendCode') }}</Button
      >
    </form>

    <form v-else class="grid gap-5" novalidate @submit.prevent="handleConfirmEmailChange">
      <p class="text-sm text-ib-text-subtle">
        {{ t('account.security.otpSentTo', { email: emailForm.newEmail }) }}
      </p>
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
        :label="t('account.security.emailOtp')"
        :error="emailErrors.otp"
        field-id="email-otp"
        required
        inline-required
        :field-class="passwordFieldClass"
      >
        <Input
          :id="fieldId"
          v-model="emailForm.otp"
          inputmode="numeric"
          autocomplete="one-time-code"
          maxlength="6"
          :required="required"
          :class="[passwordInputClass, inlineError ? passwordInputErrorClass : '']"
          :invalid="invalid"
          :placeholder="inlineError ? inlineErrorText : t('account.security.otpPlaceholder')"
          :aria-invalid="ariaInvalid"
          :aria-describedby="describedbyId"
        />
      </IbFormField>
      <Button
        type="submit"
        :loading="emailLoading"
        :disabled="emailLoading"
        :class="accountPrimaryActionButtonClass"
        >{{
          emailLoading ? t('account.security.updating') : t('account.security.confirmEmailChange')
        }}</Button
      >
    </form>
  </AccountPane>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Button } from '@/components/ui/button'
import { Input, PasswordInput } from '@/components/ui/input'
import {
  ibFormFieldClass,
  ibFormInputClass,
  ibFormInputErrorClass,
} from '../../../../shared/theme/formControls'
import IbFormField from '../../../../shared/components/IbFormField.vue'
import { useUserStore } from '../../../auth/session/userSession'
import { useAsyncAction } from '../../../../shared/composables/useAsyncAction'
import { useAppToast } from '../../../../shared/composables/useAppToast'
import { userFacingErrorCopy } from '../../../../shared/errors/errorCatalog'
import { usePasswordChange } from '../../application/security/usePasswordChange'
import AccountPane from '../common/AccountPane.vue'
import { accountPrimaryActionButtonClass } from '../common/ui'

const { t } = useI18n()
const {
  form: pwdForm,
  errors: pwdErrors,
  loading: pwdLoading,
  validateField: validatePasswordField,
  clearFieldError: clearPasswordFieldError,
  handleNewPasswordInput,
  submit: handleUpdatePassword,
} = usePasswordChange()

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
    : t('account.security.errors.enterCurrentPassword')
  emailErrors.value.newEmail = /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(emailForm.value.newEmail.trim())
    ? ''
    : t('account.security.errors.invalidEmail')
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
        success: () => appToast.success(t('account.security.toast.verificationCodeSent')),
        error: (error) =>
          appToast.error(userFacingErrorCopy(error, t('account.security.toast.sendCodeFailed'))),
      },
    },
  )
  if (sent) emailOtpSent.value = true
}

async function handleConfirmEmailChange() {
  emailErrors.value.otp = /^\d{6}$/.test(emailForm.value.otp.trim())
    ? ''
    : t('account.security.errors.sixDigitCode')
  if (emailErrors.value.otp) return
  const changed = await runAsyncAction(
    () => userStore.changeEmailWithOTP(emailForm.value.newEmail, emailForm.value.otp),
    {
      loading: emailLoading,
      feedback: {
        success: () => appToast.success(t('account.security.toast.emailUpdated')),
        error: (error) =>
          appToast.error(userFacingErrorCopy(error, t('account.security.toast.emailUpdateFailed'))),
      },
    },
  )
  if (changed) {
    emailOtpSent.value = false
    emailForm.value = { currentPassword: '', newEmail: '', otp: '' }
  }
}
</script>
