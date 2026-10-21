<template>
  <section
    :class="cn(accountCardPanelClass, 'p-4')"
    :aria-label="t('workspace.switcher.handover.title')"
  >
    <h3 class="m-0 text-admin-body font-semibold text-foreground">
      {{ t('workspace.switcher.handover.title') }}
    </h3>
    <div v-if="section.isLoading.value" class="mt-3 flex items-center gap-2" role="status">
      <Spinner class="size-4" />
      <span class="text-xs text-muted-foreground">{{ t('common.actions.loading') }}</span>
    </div>
    <LoadErrorPanel
      v-else-if="section.hasLoadError.value"
      class="mt-3"
      :message="t('workspace.switcher.handover.loadFailed')"
      :retry-label="t('common.actions.retry')"
      @retry="section.retry()"
    />
    <template v-else-if="section.account.value">
      <p class="mb-0 mt-2 break-all text-xs text-foreground">
        {{
          t('workspace.switcher.handover.account', {
            account: section.account.value.billingAccountId,
          })
        }}
      </p>
      <p class="mb-0 mt-1 text-xs leading-normal text-muted-foreground">
        {{ t('workspace.switcher.handover.scopeNote') }}
      </p>
      <div v-if="section.canInitiate.value" class="mt-4 flex flex-col gap-2">
        <label :for="recipientInputId" class="text-xs font-medium text-foreground">
          {{ t('workspace.switcher.handover.recipient') }}
        </label>
        <select
          :id="recipientInputId"
          v-model="section.recipientId.value"
          class="h-9 w-full rounded-control border border-input bg-background px-3 text-sm text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          :disabled="section.isSaving.value"
        >
          <option value="">{{ t('workspace.switcher.handover.pickRecipient') }}</option>
          <option
            v-for="member in section.recipients.value"
            :key="member.userId"
            :value="member.userId"
          >
            {{ section.memberName(member.userId) }}
          </option>
        </select>
        <p class="m-0 text-xs text-muted-foreground">
          {{ t('workspace.switcher.handover.recipientHint') }}
        </p>
        <Button
          type="button"
          size="sm"
          class="self-start"
          :is-disabled="!section.canInvite.value || section.isSaving.value"
          :is-loading="section.isSaving.value"
          @click="section.initiate()"
          >{{ t('workspace.switcher.handover.initiate') }}</Button
        >
      </div>
      <ul
        v-if="section.invites.value.length"
        class="mb-0 mt-4 list-none space-y-3 p-0"
        :aria-label="t('workspace.switcher.handover.invites')"
      >
        <li
          v-for="invite in section.invites.value"
          :key="invite.publicId"
          class="rounded-control border border-border p-3"
        >
          <p class="m-0 text-xs break-all text-foreground">
            {{
              t('workspace.switcher.handover.inviteParties', {
                from: section.memberName(invite.fromUserId),
                to: section.memberName(invite.toUserId),
              })
            }}
          </p>
          <p class="mb-0 mt-1 text-xs text-muted-foreground">
            {{ t(`workspace.switcher.handover.status.${invite.status}`) }}
          </p>
          <p v-if="invite.status === 'accepted'" class="mb-0 mt-1 text-xs text-muted-foreground">
            {{
              t('workspace.switcher.handover.grantedScope', {
                count: invite.scopeWorkspaceIds.length,
              })
            }}
          </p>
          <div v-if="invite.status === 'pending'" class="mt-2 flex flex-wrap gap-2">
            <Button
              v-if="section.canDecide(invite, 'accept')"
              type="button"
              size="sm"
              :is-disabled="section.isSaving.value"
              @click="section.requestDecision(invite, 'accept')"
            >
              {{ t('workspace.switcher.handover.accept') }}
            </Button>
            <Button
              v-if="section.canDecide(invite, 'reject')"
              type="button"
              size="sm"
              variant="outline"
              :is-disabled="section.isSaving.value"
              @click="section.requestDecision(invite, 'reject')"
            >
              {{ t('workspace.switcher.handover.reject') }}
            </Button>
            <Button
              v-if="section.canDecide(invite, 'cancel')"
              type="button"
              size="sm"
              variant="outline"
              :is-disabled="section.isSaving.value"
              @click="section.requestDecision(invite, 'cancel')"
            >
              {{ t('workspace.switcher.handover.cancel') }}
            </Button>
          </div>
        </li>
      </ul>
      <p v-else class="mb-0 mt-3 text-xs text-muted-foreground">
        {{ t('workspace.switcher.handover.empty') }}
      </p>
      <div v-if="section.managers.value.length" class="mt-4 border-t border-border pt-3">
        <h4 class="m-0 text-sm font-medium text-foreground">
          {{ t('workspace.switcher.handover.managers') }}
        </h4>
        <p class="mb-0 mt-1 text-xs text-muted-foreground">
          {{ t('workspace.switcher.handover.detachNote') }}
        </p>
        <ul class="mb-0 mt-2 list-none space-y-2 p-0">
          <li
            v-for="manager in section.managers.value"
            :key="manager.userId"
            class="flex items-center justify-between gap-2"
          >
            <span class="min-w-0 break-all text-xs text-foreground">
              {{ section.memberName(manager.userId) }} ·
              {{
                t(`workspace.switcher.members.role${manager.role === 'owner' ? 'Owner' : 'Admin'}`)
              }}
            </span>
            <Button
              v-if="section.canDetach(manager)"
              type="button"
              size="sm"
              variant="outline"
              :is-disabled="section.isSaving.value"
              @click="section.requestDetach(manager)"
              >{{
                t(
                  manager.userId === props.currentUserId
                    ? 'workspace.switcher.handover.leave'
                    : 'workspace.switcher.handover.detach',
                )
              }}</Button
            >
          </li>
        </ul>
      </div>
    </template>
    <p v-if="section.actionError.value" class="mb-0 mt-3 text-xs text-destructive" role="alert">
      {{ section.actionError.value }}
    </p>
  </section>
  <UserConfirmDialog
    :is-visible="section.confirmation.value != null"
    :title="confirmationTitle"
    :message="confirmationMessage"
    :confirm-label="confirmationTitle"
    :intent="section.confirmation.value?.kind === 'accept' ? 'primary' : 'danger'"
    :is-loading="section.isSaving.value"
    @update:is-visible="section.dismissConfirmation()"
    @confirm="section.confirm()"
  />
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'
import { cn } from '@/lib/utils'
import LoadErrorPanel from '../../../../core/components/LoadErrorPanel.vue'
import UserConfirmDialog from '../../../../core/components/UserConfirmDialog.vue'
import { useBillingTransferSection } from '../../session/billing-transfer/useBillingTransferSection'
import { accountCardPanelClass } from '../shared/ui'

const props = defineProps<{ workspaceId: string; currentUserId?: string | null | undefined }>()
const { t } = useI18n()
const section = useBillingTransferSection({
  workspaceId: () => props.workspaceId,
  currentUserId: () => props.currentUserId,
})
const recipientInputId = computed(() => `billing-transfer-recipient-${props.workspaceId}`)
const confirmationTitle = computed(() => {
  const pending = section.confirmation.value
  if (pending?.kind === 'detach' && pending.member.userId === props.currentUserId) {
    return t('workspace.switcher.handover.leave')
  }
  return t(`workspace.switcher.handover.${pending?.kind ?? 'accept'}`)
})
const confirmationMessage = computed(() => {
  const action = section.confirmation.value
  return action?.kind === 'detach'
    ? t('workspace.switcher.handover.detachConfirm', {
        member: section.memberName(action.member.userId),
      })
    : t('workspace.switcher.handover.decisionConfirm', {
        account: section.account.value?.billingAccountId ?? '',
      })
})
</script>
