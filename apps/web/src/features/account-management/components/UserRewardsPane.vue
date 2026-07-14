<template>
  <AccountManagementPane title="奖励中心">
    <section
      class="rounded-[var(--radius-ib-panel)] border border-ib-border-soft bg-ib-surface p-5 shadow-[var(--shadow-ib-control)] max-[640px]:p-4"
      aria-labelledby="reward-code-heading"
    >
      <form class="grid gap-3" @submit.prevent="submitCode">
        <label
          id="reward-code-heading"
          for="account-reward-code"
          class="text-[13px] font-semibold text-ib-text-muted"
        >
          输入兑换码
        </label>
        <div class="grid min-w-0 grid-cols-[minmax(0,1fr)_112px] gap-3 max-[640px]:grid-cols-1">
          <InputText
            id="account-reward-code"
            v-model="code"
            unstyled
            type="text"
            autocomplete="off"
            autocapitalize="characters"
            :spellcheck="false"
            :class="ibFormInputClass"
            placeholder="请输入兑换码"
          />
          <Button
            unstyled
            type="submit"
            label="兑换"
            :disabled="!canSubmit"
            :class="accountPrimaryActionButtonClass"
            :pt="accountButtonPt"
          />
        </div>
      </form>
    </section>

    <section class="grid min-w-0 gap-3" aria-labelledby="reward-history-heading">
      <h3 id="reward-history-heading" class="m-0 text-lg font-bold text-ib-text">兑换记录</h3>
      <div
        class="flex min-h-32 items-center justify-center gap-2 rounded-[var(--radius-ib-panel)] border border-dashed border-ib-border bg-ib-surface px-5 py-8 text-sm font-semibold text-ib-text-subtle"
      >
        <Gift class="size-4" aria-hidden="true" />
        <span>暂无兑换记录</span>
      </div>
    </section>
  </AccountManagementPane>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { Gift } from '@iconoir/vue'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import { ibFormInputClass } from '../../../shared/theme/formControls'
import { useProfileFeedback } from '../useProfileFeedback'
import AccountManagementPane from './AccountManagementPane.vue'
import { accountButtonPt, accountPrimaryActionButtonClass } from './accountManagementUi'

const code = ref('')
const profileFeedback = useProfileFeedback()
const normalizedCode = computed(() => code.value.trim())
const canSubmit = computed(() => normalizedCode.value.length > 0)

function submitCode() {
  if (!canSubmit.value) return
  profileFeedback.rewardRedemptionUnavailable()
}
</script>
