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
        :aria-expanded="isManageOpen"
        @click="toggleManage"
      >
        {{ isManageOpen ? t('workspace.limits.manageHide') : t('workspace.limits.manageShow') }}
      </Button>
    </div>

    <!-- Usage overview: every member reads these rows. Management forms stay
      behind the local toggle for managers only (backend enforces 403).
      Personal collaboration (#952): collaborator consumption charges the
      holder's shared Personal account with the real operator on each record;
      one (account, user) quota aggregates that member across all workspaces
      of the account. New members default to unlimited; invites never gate
      on a default (#953). Organization Owner is always unlimited (Owner sets
      Admin, Owner/Admin set Member). -->

    <p class="m-0 mt-3 text-admin-meta leading-normal text-muted-foreground" aria-live="polite">
      {{ t('workspace.quota.sharedAccountNote') }}
    </p>

    <!-- Member cross-workspace quota: self usage -->
    <div
      v-if="quota.isSelfLoading.value"
      class="mt-3 flex min-h-24 flex-col items-center justify-center gap-2"
      aria-live="polite"
    >
      <Spinner class="size-6 text-primary" />
      <span class="text-admin-meta text-muted-foreground">
        {{ t('common.actions.loading') }}
      </span>
    </div>

    <div
      v-else-if="quota.hasSelfError.value"
      class="mt-3 flex min-h-24 flex-col items-center justify-center gap-2"
      aria-live="polite"
    >
      <Button
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
        {{ t('workspace.quota.remaining', { remaining: quota.selfQuota.value.remainingCredits }) }}
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
        class="m-0 mt-2 text-admin-meta leading-normal text-destructive"
        role="alert"
      >
        {{ t('workspace.quota.exhausted') }}
      </p>
      <p v-if="!quota.canManage.value" class="m-0 mt-2 text-admin-meta text-muted-foreground">
        {{ t('workspace.quota.managerOnly') }}
      </p>
    </div>

    <!-- Management forms (managers only, behind the local toggle) -->
    <template v-if="isManageOpen && quota.canManage.value">
      <!-- Member quota list + editor -->
      <div class="mt-3 flex flex-col gap-2">
        <div
          v-if="quota.isListLoading.value"
          class="flex min-h-16 flex-col items-center justify-center gap-2"
          aria-live="polite"
        >
          <Spinner class="size-5 text-primary" />
        </div>

        <div
          v-else-if="quota.hasListError.value"
          class="flex min-h-16 flex-col items-center justify-center gap-2"
          aria-live="polite"
        >
          <Button
            type="button"
            variant="outline"
            size="sm"
            class="h-8 shrink-0 px-3"
            @click="quota.retryList()"
          >
            {{ t('common.actions.retry') }}
          </Button>
        </div>

        <div v-else class="flex flex-col gap-2">
          <p
            v-if="quota.quotas.value.length === 0"
            class="m-0 text-admin-meta text-muted-foreground"
          >
            {{ t('workspace.quota.noQuotas') }}
          </p>
          <ul v-else class="m-0 flex list-none flex-col gap-2 p-0">
            <li
              v-for="row in quota.quotas.value"
              :key="row.userId"
              class="flex items-center justify-between gap-2"
            >
              <div class="min-w-0">
                <p
                  class="m-0 truncate text-admin-meta font-medium text-foreground"
                  :title="row.userId"
                >
                  {{ quota.memberLabel(row.userId) }}
                </p>
                <p class="m-0 mt-0.5 text-admin-meta text-muted-foreground">
                  <span>
                    <template v-if="row.limitCredits != null">
                      {{
                        t('workspace.quota.usedOf', {
                          used: row.usedCredits,
                          limit: row.limitCredits,
                        })
                      }}
                    </template>
                    <template v-else>
                      {{ t('workspace.quota.usedUnlimited', { used: row.usedCredits }) }}
                    </template>
                  </span>
                  <span>{{ periodLabel(row.period) }}</span>
                </p>
              </div>
              <Button
                v-if="row.period !== 'unlimited'"
                type="button"
                variant="outline"
                size="sm"
                class="h-8 shrink-0 px-3"
                :is-disabled="quota.isSaving.value"
                @click="quota.clearQuota(row.userId)"
              >
                {{ t('workspace.quota.setUnlimited') }}
              </Button>
            </li>
          </ul>

          <div class="mt-1 flex flex-col gap-2">
            <label :for="quotaMemberSelectId" class="text-admin-meta font-medium text-foreground">
              {{ t('workspace.quota.memberLabel') }}
            </label>
            <!-- eslint-disable-next-line vuejs-accessibility/form-control-has-label -- The trigger carries its accessible name via :aria-label; the rule cannot see through the Select primitive. -->
            <Select
              :model-value="quota.targetUserId.value"
              :disabled="quota.isSaving.value"
              @update:model-value="quota.pickTargetUser($event)"
            >
              <SelectTrigger
                :id="quotaMemberSelectId"
                size="sm"
                class="w-full cursor-pointer gap-1.5"
                :aria-label="t('workspace.quota.memberLabel')"
              >
                <SelectValue :placeholder="t('workspace.quota.pickMember')">
                  {{ selectedMemberLabel }}
                </SelectValue>
                <SelectIcon>
                  <NavArrowDown class="size-3.5 shrink-0" aria-hidden="true" />
                </SelectIcon>
              </SelectTrigger>
              <SelectContent
                position="popper"
                align="start"
                :side-offset="appOverlayMenuSideOffset"
              >
                <SelectViewport>
                  <SelectItem
                    v-for="userId in quota.memberOptions.value"
                    :key="userId"
                    :value="userId"
                  >
                    <SelectItemText>{{ quota.memberLabel(userId) }}</SelectItemText>
                  </SelectItem>
                </SelectViewport>
              </SelectContent>
            </Select>

            <label :for="quotaPeriodSelectId" class="text-admin-meta font-medium text-foreground">
              {{ t('workspace.quota.periodLabel') }}
            </label>
            <!-- eslint-disable-next-line vuejs-accessibility/form-control-has-label -- The trigger carries its accessible name via :aria-label; the rule cannot see through the Select primitive. -->
            <Select
              :model-value="quota.targetPeriod.value"
              :disabled="quota.isSaving.value"
              @update:model-value="quota.pickTargetPeriod($event)"
            >
              <SelectTrigger
                :id="quotaPeriodSelectId"
                size="sm"
                class="w-full cursor-pointer gap-1.5"
                :aria-label="t('workspace.quota.periodLabel')"
              >
                <SelectValue>{{ periodLabel(quota.targetPeriod.value) }}</SelectValue>
                <SelectIcon>
                  <NavArrowDown class="size-3.5 shrink-0" aria-hidden="true" />
                </SelectIcon>
              </SelectTrigger>
              <SelectContent
                position="popper"
                align="start"
                :side-offset="appOverlayMenuSideOffset"
              >
                <SelectViewport>
                  <SelectItem value="daily">
                    <SelectItemText>{{ t('workspace.quota.periodDaily') }}</SelectItemText>
                  </SelectItem>
                  <SelectItem value="weekly">
                    <SelectItemText>{{ t('workspace.quota.periodWeekly') }}</SelectItemText>
                  </SelectItem>
                  <SelectItem value="monthly">
                    <SelectItemText>{{ t('workspace.quota.periodMonthly') }}</SelectItemText>
                  </SelectItem>
                  <SelectItem value="fixed">
                    <SelectItemText>{{ t('workspace.quota.periodFixed') }}</SelectItemText>
                  </SelectItem>
                  <SelectItem value="unlimited">
                    <SelectItemText>{{ t('workspace.quota.periodUnlimited') }}</SelectItemText>
                  </SelectItem>
                </SelectViewport>
              </SelectContent>
            </Select>

            <template v-if="quota.targetPeriod.value !== 'unlimited'">
              <label :for="quotaLimitInputId" class="text-admin-meta font-medium text-foreground">
                {{ t('workspace.quota.limitLabel') }}
              </label>
              <!-- eslint-disable-next-line vuejs-accessibility/form-control-has-label -- The limit input carries its accessible name via aria-label; the rule cannot see through the Input primitive. -->
              <Input
                :id="quotaLimitInputId"
                v-model="quota.limitDraft.value"
                size="sm"
                class="h-9 min-w-0 flex-1 bg-background text-admin-meta"
                :disabled="quota.isSaving.value"
                :aria-label="t('workspace.quota.limitLabel')"
                inputmode="numeric"
                @keydown.enter="quota.saveQuota()"
              />
            </template>
            <div class="flex shrink-0 gap-2">
              <Button
                type="button"
                size="sm"
                class="h-9 px-4"
                :is-loading="quota.isSaving.value"
                :is-disabled="quota.isSaving.value"
                @click="quota.saveQuota()"
              >
                {{ quota.isSaving.value ? t('workspace.quota.saving') : t('workspace.quota.save') }}
              </Button>
            </div>
          </div>
          <!-- 客户端字段级校验（未选成员/配额非法）仍行内提示；服务端持久化
            失败只走 toast（见 useMemberQuotaSection.persistQuota）。 -->
          <p
            v-if="quota.saveError.value"
            class="m-0 text-admin-meta leading-normal text-destructive"
            role="alert"
          >
            {{ quota.saveError.value }}
          </p>
        </div>
      </div>
    </template>
  </section>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { NavArrowDown } from '@iconoir/vue'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectIcon,
  SelectItem,
  SelectItemText,
  SelectTrigger,
  SelectValue,
  SelectViewport,
} from '@/components/ui/select'
import { Spinner } from '@/components/ui/spinner'
import { useFailureToast } from '@/core/composables/useFailureToast'
import { appOverlayMenuSideOffset } from '@/theme/appOverlayMenu'
import { useMemberQuotaSection } from '@/features/account'
import {
  quotaPeriodLabelKey,
  type MemberQuotaPeriod,
} from '@/features/account/session/quota/resource'
import { accountCardPanelClass } from '@/features/account'

const props = withDefaults(
  defineProps<{
    workspaceId: string
    canManage?: boolean | undefined
    isExpanded?: boolean | undefined
  }>(),
  {
    canManage: false,
    isExpanded: true,
  },
)

const { t } = useI18n()

// The management toggle only flips local visibility (never persisted,
// never optimistic); the gate itself reuses the quota controller's
// server-resolved canManage capability.
const isManageOpen = ref(false)

const quota = useMemberQuotaSection({
  workspaceId: () => props.workspaceId,
  canManage: () => props.canManage,
  isExpanded: () => props.isExpanded,
  isListEnabled: () => isManageOpen.value,
})

useFailureToast(
  () => quota.hasSelfError.value,
  () => t('workspace.quota.loadFailed'),
)
useFailureToast(
  () => quota.hasListError.value,
  () => t('workspace.quota.loadFailed'),
)

function toggleManage(): void {
  isManageOpen.value = !isManageOpen.value
}

const quotaMemberSelectId = `member-quota-member-${props.workspaceId}`
const quotaPeriodSelectId = `member-quota-period-${props.workspaceId}`
const quotaLimitInputId = `member-quota-limit-${props.workspaceId}`

function periodLabel(period: MemberQuotaPeriod): string {
  return t(quotaPeriodLabelKey(period))
}

const selectedMemberLabel = computed(() => {
  const userId = quota.targetUserId.value.trim()
  if (!userId) return ''
  return quota.memberLabel(userId)
})
</script>
