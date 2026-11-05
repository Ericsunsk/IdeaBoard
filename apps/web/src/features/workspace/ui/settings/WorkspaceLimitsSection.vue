<template>
  <section :class="accountCardPanelClass" :aria-label="t('workspace.limits.title')">
    <div class="flex items-center justify-between gap-2">
      <h3 class="m-0 text-admin-body font-semibold text-foreground">
        {{ t('workspace.limits.title') }}
      </h3>
      <Button
        v-if="quota.canManage.value"
        type="button"
        variant="outline"
        size="sm"
        class="h-8 shrink-0 px-3"
        :disabled="isListLoading || hasListError"
        @click="quota.openCreateDialog"
      >
        {{ t(tableKeys.createAction) }}
      </Button>
    </div>

    <!-- Usage overview: Org-only card (ADR-0042 §2: Personal carries no quota
      concept, only Organization scopes aggregate by (Organization Billing
      Account, User) with Owner always unlimited). Every member reads these
      rows; management UI (overview totals, quota table, adjust entry) is
      managers-only; backend enforces 403. New members default to unlimited;
      invites never gate on a default. Owner sets Admin, Owner/Admin set
      Member. -->

    <p class="m-0 mt-2 truncate text-admin-meta leading-body text-muted-foreground">
      {{ t('workspace.quota.sharedAccountNote') }}
    </p>

    <!-- Non-managers keep the read-only self usage block (the member list is
      a manager-only read; members read only their own row). -->
    <template v-if="!quota.canManage.value">
      <div
        v-if="quota.isSelfLoading.value || quota.hasSelfError.value"
        class="mt-3 flex min-h-24 flex-col items-center justify-center gap-2"
        aria-live="polite"
      >
        <template v-if="quota.isSelfLoading.value">
          <Spinner class="size-6 text-primary" />
          <span class="text-admin-meta text-muted-foreground">
            {{ t('common.actions.loading') }}
          </span>
        </template>
        <Button
          v-else
          type="button"
          variant="outline"
          size="sm"
          class="h-8 shrink-0 px-3"
          @click="quota.retrySelf()"
        >
          {{ t('common.actions.retry') }}
        </Button>
      </div>

      <div v-else-if="quota.selfQuota.value" class="mt-3 flex min-w-0 flex-col gap-3">
        <div class="flex items-center justify-between gap-2">
          <h4 class="m-0 text-admin-body font-semibold text-foreground">
            {{ t('workspace.quota.title') }}
          </h4>
          <span class="text-admin-meta text-muted-foreground">
            {{ periodLabel(quota.selfQuota.value.period) }}
          </span>
        </div>

        <p class="m-0 mt-2 text-admin-content text-foreground" aria-live="polite">
          <template v-if="quota.selfQuota.value.limitCredits != null">
            {{
              t('workspace.quota.usedOf', {
                used: quota.selfQuota.value.usedCredits,
                limit: quota.selfQuota.value.limitCredits,
              })
            }}
          </template>
          <template v-else>
            {{ t('workspace.quota.usedUnlimited', { used: quota.selfQuota.value.usedCredits }) }}
          </template>
        </p>
        <p
          v-if="quota.selfQuota.value.remainingCredits != null"
          class="m-0 mt-0.5 text-admin-meta text-muted-foreground"
        >
          {{
            t('workspace.quota.remaining', { remaining: quota.selfQuota.value.remainingCredits })
          }}
        </p>
        <p
          v-if="quota.selfQuota.value.periodStart && quota.selfQuota.value.periodEnd"
          class="m-0 mt-0.5 text-admin-meta text-muted-foreground"
        >
          {{
            t('workspace.quota.periodRange', {
              start: quota.selfQuota.value.periodStart,
              end: quota.selfQuota.value.periodEnd,
            })
          }}
        </p>
        <p
          v-if="quota.isSelfExhausted.value"
          class="m-0 mt-2 text-admin-meta leading-body text-destructive"
          role="alert"
        >
          {{ t('workspace.quota.exhausted') }}
        </p>
        <p class="m-0 mt-2 text-admin-meta text-muted-foreground">
          {{ t('workspace.quota.managerOnly') }}
        </p>
      </div>
    </template>

    <!-- Managers: overview totals, member usage table, adjust entry. -->
    <template v-else>
      <div
        v-if="isOverviewLoading"
        class="mt-3"
        role="status"
        :aria-label="t('common.actions.loading')"
      >
        <Skeleton class="h-4 w-24" />
      </div>
      <!-- Overview keeps only the non-derivable alert: exhausted count is the
        one signal worth surfacing above the table (managed/used totals repeat
        table rows). Hidden when zero so the card stays a single container. -->
      <p
        v-else-if="overview.exhaustedCount > 0"
        class="m-0 mt-3 text-admin-meta leading-body text-destructive"
        aria-live="polite"
      >
        {{ t(tableKeys.overviewExhausted, { count: overview.exhaustedCount }) }}
      </p>

      <AdminAlert v-if="hasListError" severity="error" class="mt-3">
        <div class="flex items-center justify-between gap-2">
          <span>{{ t('workspace.quota.loadFailed') }}</span>
          <Button
            type="button"
            variant="outline"
            size="sm"
            class="h-8 shrink-0 px-3"
            @click="quota.retryList"
          >
            {{ t('common.actions.retry') }}
          </Button>
        </div>
      </AdminAlert>

      <AdminTable
        v-if="isTableVisible"
        :value="rows"
        data-key="userId"
        scrollable
        sticky-header
        :scroll-aria-label="t(tableKeys.tableLabel)"
        :table-aria-label="t(tableKeys.tableLabel)"
        :loading="isListLoading"
        :skeleton-rows="3"
        :row-disabled="isOwnerRow"
        frame-class="mt-3"
      >
        <template #empty>
          <AdminEmptyState
            :title="t(tableKeys.emptyTitle)"
            :description="t(tableKeys.emptyDescription)"
          >
            <template #actions>
              <Button
                type="button"
                size="sm"
                :disabled="isListLoading || hasListError"
                @click="quota.openCreateDialog"
              >
                {{ t(tableKeys.createFirst) }}
              </Button>
            </template>
          </AdminEmptyState>
        </template>

        <AdminColumn
          field="userId"
          :header="t(tableKeys.headerMember)"
          :style="{ minWidth: '160px' }"
        >
          <template #body="{ data }">
            <p
              class="m-0 truncate text-admin-meta font-medium text-foreground"
              :title="data.userId"
            >
              {{ data.label }}
            </p>
          </template>
        </AdminColumn>

        <AdminColumn field="period" :header="t(tableKeys.headerPeriod)" :style="{ width: '80px' }">
          <template #body="{ data }">
            {{ periodLabel(data.period) }}
          </template>
        </AdminColumn>

        <AdminColumn
          field="usedCredits"
          :header="t(tableKeys.headerUsedTotal)"
          :style="{ width: '96px' }"
        />

        <AdminColumn
          field="remainingCredits"
          :header="t(tableKeys.headerRemaining)"
          :style="{ width: '96px' }"
        >
          <template #body="{ data }">
            {{ data.remainingCredits ?? t(tableKeys.unlimitedDash) }}
          </template>
        </AdminColumn>

        <AdminColumn field="status" :header="t(tableKeys.headerStatus)" :style="{ width: '110px' }">
          <template #body="{ data }">
            <QuotaStatusBadge :status="data.status" />
          </template>
        </AdminColumn>

        <AdminColumn
          v-if="quota.canManage.value"
          field="actions"
          :header="t(tableKeys.headerActions)"
          :style="{ width: '80px' }"
        >
          <template #body="{ data }">
            <Button
              type="button"
              variant="outline"
              size="sm"
              class="h-8 shrink-0 px-3"
              :is-disabled="!quota.canAdjustTarget(data.userId) || quota.isSaving.value"
              :aria-label="`${t(tableKeys.adjustAction)} ${data.label}`"
              @click="quota.openEditDialog(data)"
            >
              {{ t(tableKeys.adjustAction) }}
            </Button>
          </template>
        </AdminColumn>
      </AdminTable>
    </template>
  </section>

  <MemberQuotaEditDialog
    :is-visible="quota.editor.value.isVisible"
    :mode="quota.editor.value.mode"
    :members="quota.editor.value.members"
    :member="quota.editor.value.member"
    :period="quota.editor.value.period"
    :limit="quota.editor.value.limit"
    :is-pending="quota.isSaving.value"
    :field-error="quota.editor.value.error"
    @update:is-visible="quota.closeEditDialog"
    @update:member="quota.pickTargetUser"
    @update:period="quota.pickTargetPeriod"
    @update:limit="quota.setLimitDraft"
    @submit="quota.submitEditDialog"
  />

  <!-- The session owns unlimited confirmation and the subsequent save. -->
  <ConfirmDialog
    surface="user"
    :is-visible="quota.confirmation.value.isVisible"
    :title="quota.confirmation.value.title"
    :message="quota.confirmation.value.message"
    :confirm-label="quota.confirmation.value.confirmLabel"
    :intent="quota.confirmation.value.intent"
    :is-loading="quota.confirmation.value.isLoading"
    @update:is-visible="quota.dismissConfirmation"
    @confirm="quota.confirmUnlimited()"
  />
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { Spinner } from '@/components/ui/spinner'
import ConfirmDialog from '@/core/components/ConfirmDialog.vue'
import { useMemberQuotaSettings } from '../../session/useMemberQuotaSettings'
import { AdminAlert, AdminColumn, AdminEmptyState, AdminTable } from '@/features/admin/shared'
import {
  quotaPeriodLabelKey,
  type MemberQuotaPeriod,
} from '@/features/account/session/quota/resource'
import { accountCardPanelClass } from '@/features/account'
import { buildMemberQuotaView, type MemberQuotaRow } from './memberQuotaTableColumns'
import { MEMBER_QUOTA_TABLE_KEYS } from './quotaKeys'
import QuotaStatusBadge from './QuotaStatusBadge.vue'
import MemberQuotaEditDialog from './MemberQuotaEditDialog.vue'

const props = withDefaults(
  defineProps<{
    workspaceId: string
    // Org-only contract (ADR-0042 §2): Personal carries no quota concept, so
    // callers mount this card only for Organization workspaces. Required
    // string, never null; the call-site v-if is the single guard.
    organizationId: string
    canManage?: boolean | undefined
    isExpanded?: boolean | undefined
    currentUserId?: string | null | undefined
  }>(),
  {
    canManage: false,
    isExpanded: true,
    currentUserId: null,
  },
)

const { t } = useI18n()

const tableKeys = MEMBER_QUOTA_TABLE_KEYS

const quota = useMemberQuotaSettings({
  workspaceId: () => props.workspaceId,
  organizationId: () => props.organizationId,
  canManage: () => props.canManage,
  isExpanded: () => props.isExpanded,
  currentUserId: () => props.currentUserId,
})

const isListLoading = quota.isListLoading
const hasListError = quota.hasListError

const view = computed(() =>
  buildMemberQuotaView(
    quota.quotas.value,
    (userId) => quota.memberLabel(userId),
    (userId) => quota.memberRole(userId),
  ),
)

const rows = computed(() => view.value.rows)

const overview = computed(() => ({ exhaustedCount: view.value.exhaustedCount }))

const isOverviewLoading = computed(() => isListLoading.value && quota.quotas.value.length === 0)

const isTableVisible = computed(() => !hasListError.value || rows.value.length > 0)

function isOwnerRow(row: MemberQuotaRow): boolean {
  return row.role === 'owner'
}

function periodLabel(period: MemberQuotaPeriod): string {
  return t(quotaPeriodLabelKey(period))
}
</script>
