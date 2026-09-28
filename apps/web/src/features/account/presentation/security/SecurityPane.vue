<template>
  <AccountPane :title="t('account.tabs.security')">
    <form class="grid gap-5" novalidate @submit.prevent="handleUpdatePassword">
      <PasswordField
        v-model="pwdForm.oldPassword"
        :label="t('account.security.currentPassword')"
        :error="pwdErrors.oldPassword"
        field-id="profile-old-password"
        :field-class="passwordFieldClass"
        :input-class="passwordInputClass"
        :input-error-class="passwordInputErrorClass"
        autocomplete="current-password"
        :placeholder="t('account.security.currentPasswordPlaceholder')"
        @blur="validatePasswordField('oldPassword')"
        @update:model-value="clearPasswordFieldError('oldPassword')"
      />

      <div class="grid grid-cols-1 gap-5 md:grid-cols-2">
        <PasswordField
          v-model="pwdForm.newPassword"
          :label="t('account.security.newPassword')"
          :error="pwdErrors.newPassword"
          field-id="profile-new-password"
          :field-class="passwordFieldClass"
          :input-class="passwordInputClass"
          :input-error-class="passwordInputErrorClass"
          autocomplete="new-password"
          :placeholder="t('account.security.newPasswordPlaceholder')"
          @blur="validatePasswordField('newPassword')"
          @update:model-value="handleNewPasswordInput"
        />

        <PasswordField
          v-model="pwdForm.confirmPassword"
          :label="t('account.security.confirmPassword')"
          :error="pwdErrors.confirmPassword"
          field-id="profile-confirm-password"
          :field-class="passwordFieldClass"
          :input-class="passwordInputClass"
          :input-error-class="passwordInputErrorClass"
          autocomplete="new-password"
          :placeholder="t('account.security.confirmPasswordPlaceholder')"
          @blur="validatePasswordField('confirmPassword')"
          @update:model-value="clearPasswordFieldError('confirmPassword')"
        />
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
      <PasswordField
        v-model="emailForm.currentPassword"
        :label="t('account.security.reauthCurrentPassword')"
        :error="emailErrors.currentPassword"
        field-id="email-current-password"
        :field-class="passwordFieldClass"
        :input-class="passwordInputClass"
        :input-error-class="passwordInputErrorClass"
        autocomplete="current-password"
        :placeholder="t('account.security.currentPasswordPlaceholder')"
      />
      <EmailField
        v-model="emailForm.newEmail"
        :label="t('account.security.newEmail')"
        :error="emailErrors.newEmail"
        field-id="new-email"
        :field-class="passwordFieldClass"
        :input-class="passwordInputClass"
        :input-error-class="passwordInputErrorClass"
        input-type="email"
        autocomplete="email"
        :placeholder="t('account.security.newEmailPlaceholder')"
      />
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
      <OtpField
        v-model="emailForm.otp"
        :label="t('account.security.emailOtp')"
        :error="emailErrors.otp"
        field-id="email-otp"
        :field-class="passwordFieldClass"
        :input-class="passwordInputClass"
        :input-error-class="passwordInputErrorClass"
        :placeholder="t('account.security.otpPlaceholder')"
      />
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
import {
  ibFormFieldClass,
  ibFormInputClass,
  ibFormInputErrorClass,
} from '../../../../shared/theme/formControls'
import {
  EmailField,
  OtpField,
  PasswordField,
  emailFormatError,
  otpFieldError,
  requiredFieldError,
  useAuthForm,
  useUserStore,
} from '../../../auth'
import { useAsyncAction } from '../../../../shared/composables/useAsyncAction'
import { useAppToast } from '../../../../shared/composables/useAppToast'
import { userFacingErrorCopy } from '../../../../shared/errors/errorCatalog'
import { ACCOUNT_GUIDANCE_COPY_KEYS } from '../../infrastructure'
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
const emailFormApi = useAuthForm(['currentPassword', 'newEmail', 'otp'] as const)
const emailForm = emailFormApi.values
const emailErrors = emailFormApi.errors

async function handleRequestEmailChange() {
  emailErrors.value.currentPassword = requiredFieldError(
    emailForm.value.currentPassword,
    t('account.security.errors.enterCurrentPassword'),
    { sentinel: false },
  )
  emailErrors.value.newEmail = emailFormatError(
    emailForm.value.newEmail.trim(),
    t('account.security.errors.invalidEmail'),
  )
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
          appToast.error(
            userFacingErrorCopy(
              error,
              t('account.security.toast.sendCodeFailed'),
              ACCOUNT_GUIDANCE_COPY_KEYS,
            ),
          ),
      },
    },
  )
  if (sent) emailOtpSent.value = true
}

async function handleConfirmEmailChange() {
  emailErrors.value.otp = otpFieldError(
    emailForm.value.otp,
    t('account.security.errors.sixDigitCode'),
  )
  if (emailErrors.value.otp) return
  const changed = await runAsyncAction(
    () => userStore.changeEmailWithOTP(emailForm.value.newEmail, emailForm.value.otp),
    {
      loading: emailLoading,
      feedback: {
        success: () => appToast.success(t('account.security.toast.emailUpdated')),
        error: (error) =>
          appToast.error(
            userFacingErrorCopy(
              error,
              t('account.security.toast.emailUpdateFailed'),
              ACCOUNT_GUIDANCE_COPY_KEYS,
            ),
          ),
      },
    },
  )
  if (changed) {
    emailOtpSent.value = false
    emailForm.value = { currentPassword: '', newEmail: '', otp: '' }
  }
}
</script>
