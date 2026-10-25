<template>
  <main class="grid min-h-[70vh] place-items-center px-6 py-12 text-foreground">
    <div
      class="flex w-full max-w-md flex-col items-center gap-6 rounded-overlay border border-border bg-card p-8 shadow-floating text-center"
      aria-live="polite"
    >
      <StudioName class="mb-2" />

      <!-- Loading State -->
      <template v-if="isLoading">
        <Spinner class="size-8 text-primary" />
        <p class="m-0 text-sm text-muted-foreground">{{ t('common.actions.loading') }}</p>
      </template>

      <!-- Invalid / Missing Token -->
      <template v-else-if="!token">
        <div class="flex flex-col items-center gap-2">
          <h1 class="m-0 text-lg font-bold text-foreground">
            {{ t('workspace.invite.claim.invalidTokenTitle') }}
          </h1>
          <p class="m-0 text-sm text-muted-foreground">
            {{ t('workspace.invite.claim.invalidTokenDescription') }}
          </p>
        </div>
        <Button type="button" variant="outline" class="mt-2" @click="goHome">
          {{ t('workspace.invite.claim.backToHome') }}
        </Button>
      </template>

      <!-- Error State (e.g. 404 Not Found) -->
      <template v-else-if="isError || !preview">
        <div class="flex flex-col items-center gap-2">
          <h1 class="m-0 text-lg font-bold text-foreground">
            {{ t('workspace.invite.claim.notFoundTitle') }}
          </h1>
          <p class="m-0 text-sm text-muted-foreground">
            {{ t('workspace.invite.claim.notFoundDescription') }}
          </p>
        </div>
        <Button type="button" variant="outline" class="mt-2" @click="goHome">
          {{ t('workspace.invite.claim.backToHome') }}
        </Button>
      </template>

      <!-- Expired State -->
      <template v-else-if="isExpired">
        <div class="flex flex-col items-center gap-2">
          <h1 class="m-0 text-lg font-bold text-foreground">
            {{ t('workspace.invite.claim.expiredTitle') }}
          </h1>
          <p class="m-0 text-sm text-muted-foreground">
            {{ t('workspace.invite.claim.expiredDescription') }}
          </p>
        </div>
        <Button type="button" variant="outline" class="mt-2" @click="goHome">
          {{ t('workspace.invite.claim.backToHome') }}
        </Button>
      </template>

      <!-- Already Accepted State -->
      <template v-else-if="preview.status === 'accepted' || isAccepted">
        <div class="flex flex-col items-center gap-2">
          <h1 class="m-0 text-lg font-bold text-foreground">
            {{ t('workspace.invite.claim.alreadyAcceptedTitle') }}
          </h1>
          <p class="m-0 text-sm text-muted-foreground">
            {{ t('workspace.invite.claim.alreadyAcceptedDescription') }}
          </p>
        </div>
        <Button type="button" class="mt-2" @click="goToBoards">
          {{ t('workspace.invite.claim.goToWorkspace') }}
        </Button>
      </template>

      <!-- Declined or Cancelled State -->
      <template v-else-if="preview.status === 'declined' || preview.status === 'cancelled' || isDeclined">
        <div class="flex flex-col items-center gap-2">
          <h1 class="m-0 text-lg font-bold text-foreground">
            {{ t('workspace.invite.claim.revokedTitle') }}
          </h1>
          <p class="m-0 text-sm text-muted-foreground">
            {{ t('workspace.invite.claim.revokedDescription') }}
          </p>
        </div>
        <Button type="button" variant="outline" class="mt-2" @click="goHome">
          {{ t('workspace.invite.claim.backToHome') }}
        </Button>
      </template>

      <!-- Active Pending Invitation Preview -->
      <template v-else>
        <!-- Inviter Avatar if present -->
        <Avatar v-if="preview.inviter_avatar" class="size-14 shrink-0">
          <AvatarImage :src="preview.inviter_avatar" :alt="preview.inviter_name || ''" />
          <AvatarFallback class="bg-primary/10 text-primary font-bold text-sm select-none">
            {{ (preview.inviter_name || 'W').charAt(0).toUpperCase() }}
          </AvatarFallback>
        </Avatar>

        <div class="flex flex-col items-center gap-2">
          <h1 class="m-0 text-xl font-bold text-foreground">
            {{ t('workspace.invite.claim.previewHeadline', { workspace: preview.workspace_name }) }}
          </h1>
          <p class="m-0 text-sm text-muted-foreground">
            {{
              preview.inviter_name
                ? t('workspace.invite.claim.previewSubhead', {
                    inviter: preview.inviter_name,
                    role: roleLabel(preview.role),
                  })
                : t('workspace.invite.claim.previewRole', { role: roleLabel(preview.role) })
            }}
          </p>
        </div>

        <!-- Unauthenticated: Prompt Sign In -->
        <div v-if="!isAuthenticated" class="flex flex-col w-full gap-3 mt-2">
          <Button type="button" class="w-full h-10" @click="handleSignIn">
            {{ t('workspace.invite.claim.signInToAccept') }}
          </Button>
          <p class="m-0 text-xs text-muted-foreground">
            {{ t('workspace.invite.claim.signInNote') }}
          </p>
        </div>

        <!-- Authenticated: Accept & Decline Actions -->
        <div v-else class="flex flex-col w-full gap-2.5 mt-2">
          <Button
            type="button"
            class="w-full h-10"
            :is-loading="isActionPending"
            :is-disabled="isActionPending"
            @click="handleAccept"
          >
            {{ t('workspace.invite.claim.acceptAction') }}
          </Button>
          <Button
            type="button"
            variant="outline"
            class="w-full h-10"
            :is-disabled="isActionPending"
            @click="handleDecline"
          >
            {{ t('workspace.invite.claim.declineAction') }}
          </Button>
        </div>
      </template>
    </div>
  </main>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useQuery, useQueryClient } from '@tanstack/vue-query'
import { useI18n } from 'vue-i18n'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'
import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar'
import StudioName from '../../../../core/components/StudioName.vue'
import { useAppToast } from '../../../../core/composables/useAppToast'
import { useUserStore } from '../../../auth'
import { stringParam } from '../../../../core/router/queryParams'
import {
  acceptWorkspaceInviteByToken,
  declineWorkspaceInviteByToken,
  workspaceInviteByTokenQueryOptions,
} from '../../selection/workspaceInvitesResource'
import {
  syncInviteAcceptanceQueries,
  syncInviteDeclineQueries,
} from '@/features/notification/session/useNotifications'
import { HOME_PATH, USER_BOARDS_PATH, USER_LOGIN_PATH } from '../../routePaths'

const route = useRoute()
const router = useRouter()
const { t } = useI18n()
const toast = useAppToast()
const queryClient = useQueryClient()
const userStore = useUserStore()

const token = computed(() => stringParam(route.query.token)?.trim() || '')

const inviteQuery = useQuery({
  ...workspaceInviteByTokenQueryOptions(token),
  enabled: computed(() => token.value.length > 0),
})

const preview = computed(() => inviteQuery.data.value)
const isLoading = computed(() => token.value.length > 0 && inviteQuery.isLoading.value)
const isError = computed(() => inviteQuery.isError.value)

const isExpired = computed(() => {
  if (!preview.value) return false
  if (preview.value.status === 'expired') return true
  if (preview.value.expires_at) {
    return new Date(preview.value.expires_at).getTime() <= Date.now()
  }
  return false
})

const isAuthenticated = computed(() => Boolean(userStore.user))

const isActionPending = ref(false)
const isAccepted = ref(false)
const isDeclined = ref(false)

function roleLabel(role: string): string {
  switch (role) {
    case 'owner':
      return t('workspace.switcher.members.roleOwner')
    case 'admin':
      return t('workspace.switcher.members.roleAdmin')
    default:
      return t('workspace.switcher.members.roleMember')
  }
}

function goHome() {
  void router.push(HOME_PATH)
}

function goToBoards() {
  void router.push(USER_BOARDS_PATH)
}

function handleSignIn() {
  void router.push({
    path: USER_LOGIN_PATH,
    query: { redirect: route.fullPath },
  })
}

async function handleAccept() {
  if (!token.value || isActionPending.value) return
  isActionPending.value = true
  try {
    await acceptWorkspaceInviteByToken(token.value)
    // Same set the notification session syncs on invite acceptance.
    await syncInviteAcceptanceQueries(queryClient)
    toast.success(
      t('workspace.invite.claim.acceptSuccess', {
        workspace: preview.value?.workspace_name || '',
      }),
    )
    isAccepted.value = true
    void router.push(USER_BOARDS_PATH)
  } catch {
    toast.error(t('workspace.invite.claim.acceptFailed'))
  } finally {
    isActionPending.value = false
  }
}

async function handleDecline() {
  if (!token.value || isActionPending.value) return
  isActionPending.value = true
  try {
    await declineWorkspaceInviteByToken(token.value)
    // Same set the notification session syncs on invite decline.
    await syncInviteDeclineQueries(queryClient)
    toast.success(t('workspace.invite.claim.declineSuccess'))
    isDeclined.value = true
  } catch {
    toast.error(t('workspace.invite.claim.declineFailed'))
  } finally {
    isActionPending.value = false
  }
}
</script>
