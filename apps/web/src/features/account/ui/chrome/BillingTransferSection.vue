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
      <span class="text-admin-meta text-muted-foreground">{{ t('common.actions.loading') }}</span>
    </div>
    <LoadErrorPanel
      v-else-if="section.hasLoadError.value"
      class="mt-3"
      :message="t('workspace.switcher.handover.loadFailed')"
      :retry-label="t('common.actions.retry')"
      @retry="section.retry()"
    />
    <template v-else-if="section.account.value">
      <p
        v-if="section.addressedInvite.value"
        class="mb-0 mt-3 text-admin-meta font-medium text-foreground"
      >
        {{ t('workspace.switcher.handover.recipientNotice') }}
      </p>
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
          <p class="m-0 text-admin-meta break-all text-foreground">
            {{
              t('workspace.switcher.handover.inviteParties', {
                from: section.memberName(invite.fromUserId),
                to: section.memberName(invite.toUserId),
              })
            }}
          </p>
          <p class="mb-0 mt-1 text-admin-meta text-muted-foreground">
            {{ inviteStatus(invite) }}
          </p>
          <p
            v-if="isTransferInviteLive(invite)"
            class="mb-0 mt-1 text-admin-meta text-muted-foreground"
          >
            {{ t('workspace.switcher.handover.expiresNote') }}
          </p>
          <div v-if="section.canDecide(invite, 'cancel')" class="mt-2 flex flex-wrap gap-2">
            <Button
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
      <p v-else class="mb-0 mt-3 text-admin-meta text-muted-foreground">
        {{ t('workspace.switcher.handover.empty') }}
      </p>
    </template>
    <p
      v-if="section.actionError.value"
      class="mb-0 mt-3 text-admin-meta text-destructive"
      role="alert"
    >
      {{ section.actionError.value }}
    </p>
  </section>
  <UserConfirmDialog
    :is-visible="section.confirmation.value != null"
    :title="confirmationTitle"
    :message="confirmationMessage"
    :confirm-label="confirmationTitle"
    intent="danger"
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
import {
  isTransferInviteLapsed,
  isTransferInviteLive,
  type BillingTransferInviteSummary,
} from '../../session/billing-transfer/resource'
import { useBillingTransferSection } from '../../session/billing-transfer/useBillingTransferSection'
import { accountCardPanelClass } from '../shared/ui'

const props = defineProps<{ workspaceId: string; currentUserId?: string | null | undefined }>()
const { t } = useI18n()
const section = useBillingTransferSection({
  workspaceId: () => props.workspaceId,
  currentUserId: () => props.currentUserId,
})

function inviteStatus(invite: BillingTransferInviteSummary): string {
  if (isTransferInviteLapsed(invite)) return t('workspace.switcher.handover.status.expired')
  return t(`workspace.switcher.handover.status.${invite.status}`)
}
const confirmationTitle = computed(() => {
  const pending = section.confirmation.value
  return t(`workspace.switcher.handover.${pending?.kind ?? 'cancel'}`)
})
const confirmationMessage = computed(() => {
  const pending = section.confirmation.value
  return t(`workspace.switcher.handover.${pending?.kind ?? 'cancel'}Confirm`)
})
</script>
