<template>
  <AccountManagementPane title="账号安全">
    <form class="grid gap-5" novalidate @submit.prevent="handleUpdatePassword">
      <label :class="passwordFieldClass">
        <span>当前原密码</span>
        <Password
          v-model="pwdForm.oldPassword"
          autocomplete="current-password"
          :feedback="false"
          fluid
          unstyled
          :pt="ibPasswordPt"
          :input-props="{
            autocomplete: 'current-password',
            'aria-describedby': getPasswordDescribedby('oldPassword', 'profile-old-password-error'),
          }"
          :invalid="Boolean(pwdErrors.oldPassword)"
          :input-class="[
            passwordInputClass,
            isPasswordInlineError('oldPassword') ? passwordInputErrorClass : '',
          ]"
          :placeholder="
            isPasswordInlineError('oldPassword')
              ? displayErrorMessage(pwdErrors.oldPassword)
              : '请输入当前使用的密码'
          "
          toggle-mask
          @blur="validatePasswordField('oldPassword')"
          @update:model-value="clearPasswordFieldError('oldPassword')"
        />
        <Message
          v-if="shouldShowPasswordMessage('oldPassword')"
          id="profile-old-password-error"
          unstyled
          :pt="ibFormErrorMessagePt"
          severity="error"
          size="small"
          :closable="false"
        >
          {{ displayErrorMessage(pwdErrors.oldPassword) }}
        </Message>
      </label>

      <div class="grid grid-cols-2 gap-5 max-[720px]:grid-cols-1">
        <label :class="passwordFieldClass">
          <span>设置新密码</span>
          <Password
            v-model="pwdForm.newPassword"
            autocomplete="new-password"
            :feedback="false"
            fluid
            unstyled
            :pt="ibPasswordPt"
            :input-props="{
              autocomplete: 'new-password',
              'aria-describedby': getPasswordDescribedby(
                'newPassword',
                'profile-new-password-error',
              ),
            }"
            :invalid="Boolean(pwdErrors.newPassword)"
            :input-class="[
              passwordInputClass,
              isPasswordInlineError('newPassword') ? passwordInputErrorClass : '',
            ]"
            :placeholder="
              isPasswordInlineError('newPassword')
                ? displayErrorMessage(pwdErrors.newPassword)
                : '不少于6位字符'
            "
            toggle-mask
            @blur="validatePasswordField('newPassword')"
            @update:model-value="handleNewPasswordInput"
          />
          <Message
            v-if="shouldShowPasswordMessage('newPassword')"
            id="profile-new-password-error"
            unstyled
            :pt="ibFormErrorMessagePt"
            severity="error"
            size="small"
            :closable="false"
          >
            {{ displayErrorMessage(pwdErrors.newPassword) }}
          </Message>
        </label>

        <label :class="passwordFieldClass">
          <span>重复新密码</span>
          <Password
            v-model="pwdForm.confirmPassword"
            autocomplete="new-password"
            :feedback="false"
            fluid
            unstyled
            :pt="ibPasswordPt"
            :input-props="{
              autocomplete: 'new-password',
              'aria-describedby': getPasswordDescribedby(
                'confirmPassword',
                'profile-confirm-password-error',
              ),
            }"
            :invalid="Boolean(pwdErrors.confirmPassword)"
            :input-class="[
              passwordInputClass,
              isPasswordInlineError('confirmPassword') ? passwordInputErrorClass : '',
            ]"
            :placeholder="
              isPasswordInlineError('confirmPassword')
                ? displayErrorMessage(pwdErrors.confirmPassword)
                : '请再次输入新密码'
            "
            toggle-mask
            @blur="validatePasswordField('confirmPassword')"
            @update:model-value="clearPasswordFieldError('confirmPassword')"
          />
          <Message
            v-if="shouldShowPasswordMessage('confirmPassword')"
            id="profile-confirm-password-error"
            unstyled
            :pt="ibFormErrorMessagePt"
            severity="error"
            size="small"
            :closable="false"
          >
            {{ displayErrorMessage(pwdErrors.confirmPassword) }}
          </Message>
        </label>
      </div>

      <Button
        type="submit"
        :label="pwdLoading ? '更新中...' : '确认更新密码'"
        :loading="pwdLoading"
        :disabled="pwdLoading"
        size="large"
        fluid
      />
    </form>
  </AccountManagementPane>
</template>

<script setup lang="ts">
import Button from 'primevue/button'
import Message from 'primevue/message'
import Password from 'primevue/password'
import {
  ibFormErrorMessagePt,
  ibFormFieldClass,
  ibFormInputClass,
  ibFormInputErrorClass,
  ibPasswordPt,
} from '../../../shared/theme/formControls'
import { usePasswordChangeWorkflow } from '../usePasswordChangeWorkflow'
import AccountManagementPane from './AccountManagementPane.vue'

const {
  form: pwdForm,
  errors: pwdErrors,
  loading: pwdLoading,
  validateField: validatePasswordField,
  clearFieldError: clearPasswordFieldError,
  handleNewPasswordInput,
  submit: handleUpdatePassword,
  isInlineError: isPasswordInlineError,
  shouldShowMessage: shouldShowPasswordMessage,
  getDescribedby: getPasswordDescribedby,
  displayErrorMessage,
} = usePasswordChangeWorkflow()

const passwordFieldClass = ibFormFieldClass
const passwordInputClass = ibFormInputClass
const passwordInputErrorClass = ibFormInputErrorClass
</script>
